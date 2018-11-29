fabrik
======

*Infrastructure as code.*

Fabrik is a serverless continuous integration and deployment ("CI/CD") orchestrator for services built on AWS.
It allows your service's build, test, and deployment lifecycle to be completely defined *as code*, and live right
alongside the service implementation. This allows the lifecycle to be completely automated, tested in isolation from
other deployments of the same service, and most importantly, *reliable*.

Created with :heart: at [Opolis](https://opolis.co) in Colorado.

Inspired by the ideas presented by [awesome-codepipeline-ci](https://github.com/nicolai86/awesome-codepipeline-ci)

Please note these docs are still very much a work in progress. Let me know where there are gaps, or where
more clarification is needed by opening an issue!

## Overview

TBD (new pipeline, per branch, etc)

## Getting Started

Before setting up Fabrik in your AWS account, it's important to know that it isn't designed to be a "plug-and-play"
system. For it to be used effectively, it requires having in-depth knowledge of your deployment architecture, how
various components integrate with one another, and how those components share resources. Fabrik simply provides a
framework and set of conventions to take that knowledge, and turn it into a repeatable and reliable process.

At a minimum, Fabrik assumes you are deploying to AWS, and have a basic working knowledge of CloudFormation.
If you haven't spent much time with CloudFormation, don't worry, I try to explain the high-level concepts where
appropriate in the [examples](./docs/examples.md).

The CloudFormation [Resource and Property Type Reference](https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/aws-template-resource-type-ref.html)
will be your best friend. Anything CloudFormation supports, Fabrik supports.

### Prerequisites

* [`aws`](https://aws.amazon.com/cli/) CLI utility
* [`docker`](https://docs.docker.com/install/) daemon
* `make`

### Configure `aws`

Add a profile to `$HOME/.aws/credentials` that will serve as your fabrik deployment identity in AWS. This should
be a set of access credentials that have administrator privileges. It's a good idea to rotate this key on
a regular basis. Access keys can be created at the [IAM console](https://console.aws.amazon.com/iam/home?#/users).

```
[fabrik]
aws_access_key_id = AKIA...
aws_secret_access_key = O4vew...
region = <any valid region> # i.e. us-west-2
output = json
```

### Clone

For now, the primary deployment mechanism is from a local clone of this repository.

`$ git clone git@github.com:ngmiller/fabrik.git`

### Install

Build the Docker image that will provide a runtime environment for fetching dependencies and the
[`serverless`](https://serverless.com/) deployment.

`$ make image`

### Build

Fetch dependencies

`$ make deps`

Build each Lambda function

`$ make build`

### Configure `fabrik`

Fabrik needs two secret keys to interact with GitHub. These secrets are stored on AWS SSM,
and encrypted with a key from KMS. The easiest way to set and read these keys on AWS
is to use [`fabrik-config`](./cli/config/).

|Key|Description|
|---|-----------|
|`fabrik.github.token`|GitHub OAuth token with `repo` scope|
|`fabrik.github.hmac`|GitHub HMAC key used in webhook configuration|

`fabrik.github.token`

1. [Create](https://console.aws.amazon.com/kms/home?region=us-west-2#/kms/keys/create) an encryption key on KMS
and make note of the ID (e.g. `adad8b59-c518-40e0-8039-f91fca167833`)
2. [Obtain](https://github.com/settings/tokens/new) an OAuth token from GitHub with `repo` scope.
3. Use `fabrik-config` to store it securely

```
$ fabrik-config --profile fabrik set fabrik.github.token your-aws-kms-key-id
```

`fabrik.github.hmac`

Create a random 64 character hex string to use as your webhook HMAC key. GitHub
will use this key to sign all outgoing webhook payloads, and Fabrik will use it
to validate the authenticity of the webhook by checking the signature.

Any means of doing this is satisfactory, but if you want a quick solution, copy the value
created from,

```
$ ./cli/random.sh
```

```
fabrik-config --profile fabrik set fabrik.github.hmac your-aws-kms-key-id
```

### Deploy

Deploy the entire stack defined in `serverless.yml`

`$ make deploy`

**NOTE: This can be used for updating Fabrik as well. Simply pull the latest version of the repo, and rerun this command.**

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
stage: dev
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

**Make note of the API Gateway endpoint above. This will be used to configure the webhook
endpoint on your GitHub repositories.**

## Adding a Repository

In order for a push to a GitHub repository to be processed by Fabrik, you must first
configure a webhook for your target repo and add the required files.
See [Adding a Repository](./docs/adding-a-repo.md).

## Let's Go!

Now that Fabrik has been deployed to your AWS account, you are ready to start writing some CloudFormation
templates for your services. Take a look at the [examples](./docs/examples.md) to see what's possible.

## Loose Ends

### Doesn't AWS have a product that does this already?

Yes and no. CloudFormation, CodeBuild, and CodePipeline provide the majority of the heavy lifting Fabrik
doesn't do itself, but there are certain properties I wanted my deployments to have, like unlimited isolated
deployments, that I wasn't getting with those products on their own.

According to the [CodePipeline Concepts](https://docs.aws.amazon.com/codepipeline/latest/userguide/concepts.html) document,

> Multiple revisions can be processed in the same pipeline, but **each stage can process only one revision at a time.**

This means that even if we can push several branches/revisions through a single pipeline, these revisions
cannot be processed in parallel, effectively putting a hard cap on your team's lifecycle throughput. Fabrik fixes this
by automating the provisioning of a new CodePipeline for each branch created. The only pipelines that are reused
are `staging` (for the `master` branch), and `production` (for tagged releases).

So, hopefully CodePipeline will incorporate some kind of parallel pipeline processing for the same template
in the near future, but until that happens, Fabrik will continue to fill in that gap.

### Limitations

* AWS [limits](https://docs.aws.amazon.com/codepipeline/latest/userguide/limits.html) the total number of CodePipeline
instances to 300 in any given region. If you have hundreds of developers pushing multiple branches per day for testing,
you may hit this limit. Ideally, Fabrik would utilize multiple regions at once, making this limit 1200 for all supported US
and 1200 for all supported EU regions. Let's work together to figure out a solution!

### License

Licensed under the [MIT License](./LICENSE) and available for all.

