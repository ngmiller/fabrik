`fabrik-config`
===============

`fabrik-config` is a simple CLI interface for reading and writing encrypted AWS SSM parameters.

## Install

```
$ ./install.sh
```

This will fetch the latest compiled binary from the Fabrik release on GitHub. It does
a fairly rudimentary job of checking which OS you are running, but defaults to Linux.

To check the installation, run `$ fabrik-config` if you allowed saving to `/usr/local/bin`,
otherwise, run it from this directory.

## Usage

### Options

`--profile NAME`

Use the profile `NAME` set in `$HOME/.aws/credentials`. If this option is not set,
`fabrik-config` will try to read the access and region configuration from the environment.
The conventions and variable names are the same as the `aws-cli` tool. See the
[docs](https://docs.aws.amazon.com/cli/latest/userguide/cli-environment.html) for more detail.

### `read`

Read an encrypted value from SSM. The current profile must have access to the KMS key used
to write the value.

```
$ fabrik-config read my.parameter.name
```

Multiple parameters may be fetched at once. Output will be one value per line, in the order
they were requested.

```
$ fabrik-config read my.first.parameter my.second.parameter ... my.nth.parameter
```

**WARNING: This writes decrypted values to stdout. Be aware of this when using
in a service runtime context.**

In a service context where stdout is logged, it is recommended to read the decrypted
value into an environment variable.

```
MYVAR=$(fabrik-config read my.secret)
```

### `write`

Write an encrypted value to SSM. The current profile must have access to use the given
KMS key for encryption. Arguments are pairs of `parameter-name kms-key-id`. For every
pair given, a prompt is show where you can paste the desired value.

```
$ fabrik-config write my.first.parameter 1234-my-kms-key-id ... my.last.paramter 5678-my-kms-key-id
```

Input is similar to password input, that is, you will not see the value provided echoed
back out to the terminal.

## Using with Fargate

When including this tool in a Fargate service, the service must have the following IAM statements
included in its role policy.

```
...
{
    "Effect": "Allow",
    "Action": "ssm:GetParameters",
    "Resource": [
        "arn:aws:ssm:*:*:parameter/<SSM parameter name, e.g. prod.myapp.secret>"
        ( add other parameters as necessary )
    ]
},
{
    "Effect": "Allow",
    "Action": "kms:Decrypt",
    "Resource": "arn:aws:kms:*:*:key/<SSM key id>"
}
...
```

where `<SSM key id>` is the UUID of the encryption key you chose when writing the value to SSM.

Be sure to assign your IAM task roles to the Task Definition like so,

```
"ExecutionRoleArn": { "Fn::GetAtt": [ "ECSTaskRole", "Arn" ] },
"TaskRoleArn": { "Fn::GetAtt": [ "ECSTaskRole", "Arn" ] },
```

Also, set the default region environment variable in the service's task defintion to match the
region where the SSM value exists. This is used by the AWS SDK inside this utility.

```
"ContainerDefinitions": [{
    ...
    "Environment": [
        { "Name": "AWS_DEFAULT_REGION", "Value": { "Ref": "AWS::Region" } },
        ...
    ],
    ...
```

