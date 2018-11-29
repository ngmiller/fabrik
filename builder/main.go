package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ngmiller/fabrik/lambda"
	"github.com/ngmiller/fabrik/repo"
	"github.com/ngmiller/fabrik/secure"
	"github.com/ngmiller/fabrik/stack"
	"github.com/ngmiller/fabrik/types"

	"github.com/aws/aws-lambda-go/events"
	awsLambda "github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go/aws/session"

	log "github.com/sirupsen/logrus"
)

const (
	// Execution timeout in seconds
	ExecutionTimeout = 300
)

func init() {
	log.SetFormatter(&log.JSONFormatter{DisableTimestamp: true})
}

func main() {
	awsLambda.Start(Handler)
}

// Handler serves as the integration point between the AWS event and business logic by
// preparing conrete types to satisfy the Handler's interface.
func Handler(dynamoEvent events.DynamoDBEvent) error {
	defer func() {
		if r := recover(); r != nil {
			log.Errorln("recovered from panic:", r)
		}
	}()

	// AWS session
	sess := session.Must(session.NewSession())

	for _, record := range dynamoEvent.Records {
		// skip modify and remove events from dynamo
		if record.EventName != types.DynamoDBEventInsert {
			log.Warnln("received non INSERT event from dynamo - no action")
			return nil
		}

		// parse github event
		item := record.Change.NewImage
		eventType := item["type"].String()
		rawEvent := []byte(item["payload"].String())

		if eventType != types.EventTypePush {
			log.Warnln("received non-push event:", eventType, "- no action")
			return nil
		}

		var event types.GitHubEvent
		if err := json.Unmarshal(rawEvent, &event); err != nil {
			log.Errorln("json.Unmarshal", err.Error())
			return nil
		}

		log := log.WithFields(log.Fields{
			"ref":    parseRef(event.Ref),
			"commit": shortHash(event.After),
			"repo":   event.Repository.Name,
		})

		// do nothing if branch contains NOBUILD
		if strings.Contains(event.Ref, "NOBUILD") {
			log.Warnln("received event ref requests no build - no action")
			return nil
		}

		// fetch secure repo token
		secureStore := secure.NewAWSSecureStore(sess)
		token, err := secureStore.Get(types.KeyToken)
		if err != nil {
			log.Errorln("parameter.Get", err.Error())
			return nil
		}

		// prepare processing dependencies
		stackManager := stack.NewAWSStackManager(log, sess)
		lambdaManager := lambda.NewAWSLambdaManager(sess)

		repo := repo.NewGitHubRepository(log, event.Repository.Owner.Name, event.Repository.Name, token)
		shortHash := shortHash(event.After)

		// status - pending
		repo.Status(event.After, prepStatus(types.GitStatePending, shortHash))

		// wait until we get a concrete stack status
		// or 90% of the execution timeout has been used, in which case, restart
		stop := make(chan struct{})
		status := Process(log, stop, event, repo, stackManager, token)

		select {
		case err = <-status:
			if err != nil {
				log.Errorln("error processing event:", err.Error())
				repo.Status(event.After, prepStatus(types.GitStateFailure, shortHash))
				return nil
			}
		case <-time.After(0.9 * ExecutionTimeout * time.Second):
			log.Infoln("execution timeout reached, restarting function!")
			close(stop)

			return lambdaManager.Invoke(lambdacontext.FunctionName, dynamoEvent)
		}

		// status - ok
		repo.Status(event.After, prepStatus(types.GitStateSuccess, shortHash))
	}

	return nil
}

// Process reacts to GitHub push event writes from the DynamoDB table stream
// and processes them for building. Each incoming event structure is the exact JSON from GitHub.
// We assume we are _only_ receiving push events at this time.
// Incoming refs are of the form 'ref/{heads|tag}/{value}'
//
// A repository's 'stack' in this context means an infrastructure template (i.e. CloudFormation)
// defining the CI pipeline, build and deployment resources.
//
// Each pipeline is parameterized via a parameters.json file in the repo. Each parameter set
// is keyed by 'development', 'staging', and 'production' - corresponding to the CodePipeline instance
// by the same name ('development' parameters are applied to all non master/tag refs)
//
//     if ref is tag:
//       stack = {repo}-production
//     if ref = 'master':
//       stack = {repo}-staging
//     else:
//       stack = {repo}-{ref}
//
//     if event.deleted:
//       if not exists(stack): warn and skip
//       else: delete stack
//       return
//
//     prepare context and set parameters
//
//     create or update stack with parameters
//     if tag: call UpdatePipeline with tag
//
//     monitor stack progress
//     if stack was updated:
//       start pipeline
//
func Process(log *log.Entry, stop <-chan struct{}, event types.GitHubEvent, repo types.Repository, manager types.StackManager, repoToken string) <-chan error {
	result := make(chan error)
	go func() {
		// Get stack state, delete if necessary
		stack := stackName(event.Repository.Name, event.Ref)
		exists, status, err := manager.Status(stack)
		if err != nil {
			result <- err
			return
		}

		if event.Deleted {
			if !exists {
				log.Warnln("received push/deleted event for non-existant stack")
				result <- nil
				return
			}

			result <- manager.Delete(stack)
			return
		}

		// fetch stack and parameter files from repoistory
		// pipeline.json - CI/CD pipeline stack spec
		// parameters.json - stack parameters
		context, err := buildContext(event, repo, "fabrik/pipeline.json", "fabrik/parameters.json")
		if err != nil {
			result <- err
			return
		}

		// ammend parameter list with required parameters
		context.Parameters = append(
			context.Parameters, requiredParameters(event, repoToken, os.Getenv("ARTIFACT_STORE"))...)

		// create or update stack with ref specific parameters
		if !exists {
			// create - pipeline is started automatically when created
			log.Infoln("stack create", stack)
			if err := manager.Create(stack, context.Parameters, context.PipelineTemplate); err != nil {
				result <- err
				return
			}
		} else {
			// only do an update if we aren't already in progress, otherwise, continue monitoring
			if statusComplete(status) || statusFailed(status) {
				log.Infoln("stack update", stack)
				if err := manager.Update(stack, context.Parameters, context.PipelineTemplate); err != nil {
					result <- err
					return
				}
			}
		}

		if err := Watch(log, stop, manager, stack); err != nil {
			result <- err
			return
		}

		if exists {
			log.Infoln("start build")
			if err := manager.StartBuild(stack); err != nil {
				result <- err
				return
			}
		}

		result <- nil
	}()

	return result
}

// Watch monitors the state of stack operation, returning an error if there
// was an error in that operation. This function will continue to monitor the stack in
// a loop until it receives a signal to stop from the given channel.
func Watch(log *log.Entry, stop <-chan struct{}, manager types.StackManager, stack string) error {
	for {
		select {
		case <-stop:
			log.Infoln("stack monitor received stop signal")
			return errors.New("received stop signal")
		default:
			_, status, err := manager.Status(stack)
			if err != nil {
				return err
			}

			// fail if status comes back as 'rollback' or 'failed' - something failed
			if statusRollback(status) || statusFailed(status) {
				log.Infoln("stack status", status)
				return errors.New("stack rollback or failure")
			}

			// continue waiting if stack status isn't complete
			if !statusComplete(status) {
				log.Infoln("stack status", status)
				time.Sleep(time.Second)
				continue
			}

			log.Infoln("stack status", status)
			return nil
		}
	}
}

//
// Helpers
//

func stackName(repo, ref string) string {
	if refType(ref) == types.GitRefMaster {
		return fmt.Sprintf("%s-staging", repo)
	}

	if refType(ref) == types.GitRefTag {
		return fmt.Sprintf("%s-production", repo)
	}

	return fmt.Sprintf("%s-%s", repo, parseRef(ref))
}

func shortHash(hash string) string {
	if len(hash) < 6 {
		return hash
	}

	return hash[:6]
}

func statusUrl(logGroup, logStream, shortHash string) string {
	base := fmt.Sprintf("https://%s.console.aws.amazon.com", os.Getenv("AWS_REGION"))
	path := fmt.Sprintf("/cloudwatch/home?region=%s#logEventViewer:group=%s;stream=%s;filter=%s",
		os.Getenv("AWS_REGION"),
		logGroup,
		logStream,
		shortHash,
	)

	return base + path
}

func statusComplete(status string) bool {
	return types.RegexCompleted.MatchString(status)
}

func statusRollback(status string) bool {
	return types.RegexRollback.MatchString(status)
}

func statusFailed(status string) bool {
	return types.RegexFailed.MatchString(status)
}

func parseRef(ref string) string {
	components := strings.Split(ref, "/")
	return components[len(components)-1]
}

func parseParameters(parameters []byte) (types.ParameterManifest, error) {
	var parsed types.ParameterManifest
	if err := json.Unmarshal(parameters, &parsed); err != nil {
		return parsed, err
	}

	return parsed, nil
}

func prepStatus(state, shortHash string) types.GitHubStatus {
	return types.GitHubStatus{
		State:     state,
		Context:   types.GitContextPrep,
		TargetUrl: statusUrl(lambdacontext.LogGroupName, lambdacontext.LogStreamName, shortHash),
	}
}

func requiredParameters(event types.GitHubEvent, repoToken, artifactStore string) []types.Parameter {
	stage := "development"
	branch := parseRef(event.Ref)

	if refType(event.Ref) == types.GitRefMaster {
		stage = "staging"
	}

	if refType(event.Ref) == types.GitRefTag {
		stage = "production"
		branch = "master"
	}

	return []types.Parameter{
		types.Parameter{ParameterKey: "ArtifactStore", ParameterValue: artifactStore},
		types.Parameter{ParameterKey: "RepoOwner", ParameterValue: event.Repository.Owner.Name},
		types.Parameter{ParameterKey: "RepoName", ParameterValue: event.Repository.Name},
		types.Parameter{ParameterKey: "RepoBranch", ParameterValue: branch},
		types.Parameter{ParameterKey: "RepoToken", ParameterValue: repoToken},
		types.Parameter{ParameterKey: "Stage", ParameterValue: stage},
	}
}

func refType(ref string) string {
	parsed := parseRef(ref)

	if parsed == types.GitRefMaster {
		return types.GitRefMaster
	} else if types.RegexTagRef.MatchString(parsed) {
		return types.GitRefTag
	}

	return types.GitRefBranch
}

func buildContext(event types.GitHubEvent, repo types.Repository, pipelinePath, parameterPath string) (types.BuildContext, error) {
	// pipeline template (required)
	pipelineTemplate, err := repo.Get(event.Ref, pipelinePath)
	if err != nil {
		return types.BuildContext{}, err
	}

	// parameter manifest (required)
	parameterSpec, err := repo.Get(event.Ref, parameterPath)
	if err != nil {
		return types.BuildContext{}, err
	}

	parameterManifest, err := parseParameters(parameterSpec)
	if err != nil {
		return types.BuildContext{}, err
	}

	// Default to development parameters, set staging or production accordingly
	parameters := parameterManifest.Development

	if refType(event.Ref) == types.GitRefMaster {
		parameters = parameterManifest.Staging
	}

	if refType(event.Ref) == types.GitRefTag {
		parameters = parameterManifest.Production
	}

	context := types.BuildContext{
		PipelineTemplate: pipelineTemplate,
		Parameters:       parameters,
	}

	return context, nil
}
