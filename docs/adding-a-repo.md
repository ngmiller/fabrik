Adding a Repository
===================

Repositories can be processed by Fabrik by configured a webhook and adding a few files to the repo.

## Configuring the Webhook

"Webhooks" are a means for GitHub to notify third party services that a particular event has occurred on a particular
repository. It is simply a `POST` request to a particular endpoint, containing an event. An event can be anything from
opening a pull request, to merging into master. For our purposes, we are only interested in what are known
as `push` events.

After deploying the serverless project in this repo, make note of the API Gateway endpoint,

```
Serverless: Packaging service...
Serverless: Excluding development dependencies...
Serverless: Uploading CloudFormation file to S3...
Serverless: Uploading artifacts...
Serverless: Uploading service .zip file to S3 (10.31 MB)...
Serverless: Validating template...
Serverless: Updating Stack...
Serverless: Checking Stack update progress...
..............
Serverless: Stack update finished...
Service Information
service: fabrik
stage: prod
region: us-west-2
stack: fabrik-prod
api keys:
  None
endpoints:
  POST - https://xxxxxxx.execute-api.us-west-2.amazonaws.com/dev/event <------- *
functions:
  listener: fabrik-prod-listener
  builder: fabrik-prod-builder
  notifier: fabrik-prod-notifier
Serverless: Removing old service versions...
```

On the GitHub repo page, go to "Settings" > "Webhooks" > "Add webhook". Provide the API Gateway endpoint as the hook
destination, the HMAC key set as a build system parameter during setup (`fabrik.github.hmac`),
and select "Just the `push` event". Be sure the content type is set to `application/json`.

And that's it! Check out the status updates on each commit pushed to GitHub to track that commit's
progress through the build system. But, first you need to create some files for Fabrik to use.

## Configuring Fabrik

Fabrik expects the following files to be present in the repository when it picks up
a `push` event from GitHub. These files do not have to be present in the `master` branch right away. Fabrik
will simply see them as part of your branch, allowing you to test the entire deployment lifecycle in isolation
from the rest of the work in the repository. (Yes, that means you can test changes to your build and deploy
configuration in a branch before promoting the changes to `master`!)

The following is a list of files to crate, along with their description. For now, Fabrik expects all files to live
in a directory at the root of your repo called `fabrik`. This may be configurable in the future, but this
setup will always be supported.

Concrete examples of these files can be found in [`examples`](examples.md).

### `fabrik/pipeline.json`

**Required**

Defines the entire CI/CD pipeline as a
[CloudFormation](https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/Welcome.html) template. Fabrik
will pass this template to CloudFormation so it can spin up a unique CodePipeline instance for your project.

This template is *required* to accept the following parameters. If any are missing, the prepartion phase
will fail. These parameters are provided by Fabrik at runtime, you are not responsible for specifying them,
just including and referencing them in the template.

|Key|Value|
|---|-----|
|`ArtifactStore`|Name of the S3 bucket responsible for storing pipeline artifacts|
|`RepoOwner`|GitHub repo namespace, i.e. `ngmiller`|
|`RepoName`|GitHub repo name|
|`RepoBranch`|Branch name to build|
|`RepoToken`|OAuth token with `repo` scope|
|`Stage`|Used to reference pipeline parameters `development`, `staging`, or `production`|

### `fabrik/parameters.json`

**Required**

Defines a set of parameters to configure CodePipeline, and use during a particular _invocation_ of the pipeline.
There are three invocation types, or "stages": `development`, `staging`, and `production`. The `development` invocation
occurs for every branch pushed to the repository, `staging` occurs after a merge into master, and `production` occurs
on every tag. The parameters file is keyed accordingly, and each set of parameters must be a list of objects in the form:

```
{
    "ParameterKey": "...",
    "ParameterValue": "..."
}
```

e.g.

```
{
    "development": [
        { "ParameterKey": "a", "ParameterValue": "1" }
    ],
    "staging": [
        ...
    ],
    "production": [
        ...
    ]
}
```

This mechanism allows the _pipeline itself_ be configured for a particular stage. Maybe you need to inject a Slack
notification Lambda function into your `production` deployment, but not your `development` ones. These parameters
can be used as toggles in CloudFormation to turn that functionality on or off. Or, maybe you need to use a different
build image for `staging` and `production`. Any configuration releated to the _pipeline_ can be done here.

### `fabrik/buildspec.yml`

**Optional** _Required only if you define a CodeBuild step in your `pipeline.json`. Most projects will._

Defines the build steps and commands run inside inside CodeBuild. CodeBuild reads this file, running each command in
sequence, failing the entire build if any command fails. If the build is successful, the artifacts are pushed the S3
bucket specified by `ArtifactStore`, and can be used in a subsequent stage in the pipeline.

### `fabrik/deploy.json`

**Optional** _Required only if you need a CloudFormation deployment to occur as part of your service lifecycle._

If the end result of your deployment is a CloudFormation stack creation/update, you can use this file
to define that stack.

`fabrik/pipeline.json` will contain a CloudFormation `Deploy` step, which will reference this file as it's
template. Various parameters for each stage can be configured with the files below. If you need to pass an environment
variable through to container instance defined in your `deploy.json`, or configure the CloudFormation stack
in any way, use these files.

They should be structured as JSON like so,

```
{
    "Parameters": {
        "MyParameter": "true",
        "AnotherParameter": "db.m3.medium"
    }
}
```

where `MyParameter` and `AnotherParameter` have already been defined as valid parameters in `deploy.json`

#### `fabrik/development.json`

Configures `deploy.json` during a `development` invocation.

#### `fabrik/staging.json`

Configures `deploy.json` during a `staging` invocation.

#### `fabrik/production.json`

Configures `deploy.json` during a `production` invocation.
