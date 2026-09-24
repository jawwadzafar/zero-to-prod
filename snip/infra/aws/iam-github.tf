# Let GitHub Actions deploy snip with short-lived credentials — no access keys
# stored anywhere (chapters 11.2 and 12.4).
variable "github_repository" {
  description = "owner/name of the repository allowed to deploy."
  type        = string
  default     = "jawwadzafar/zero-to-prod"
}

# Trust GitHub's OIDC token issuer (one per AWS account).
resource "aws_iam_openid_connect_provider" "github" {
  url            = "https://token.actions.githubusercontent.com"
  client_id_list = ["sts.amazonaws.com"]
}

# Who may assume the deploy role: only workflows in this repository, running
# on the main branch. Anything else — forks, other branches, pull requests —
# is refused, even with a valid GitHub token.
data "aws_iam_policy_document" "github_assume" {
  statement {
    actions = ["sts:AssumeRoleWithWebIdentity"]
    principals {
      type        = "Federated"
      identifiers = [aws_iam_openid_connect_provider.github.arn]
    }
    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }
    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:sub"
      values   = ["repo:${var.github_repository}:ref:refs/heads/main"]
    }
  }
}

resource "aws_iam_role" "github_deploy" {
  name                 = "${local.name}-github-deploy"
  assume_role_policy   = data.aws_iam_policy_document.github_assume.json
  max_session_duration = 3600 # credentials expire within an hour
}

# What the role may do: roll out new task definitions for snip's two services,
# and nothing else. Least privilege, scoped to exact resources.
data "aws_iam_policy_document" "github_deploy" {
  statement {
    sid       = "UpdateSnipServices"
    actions   = ["ecs:UpdateService", "ecs:DescribeServices"]
    resources = [aws_ecs_service.api.id, aws_ecs_service.worker.id]
  }
  statement {
    sid       = "RegisterTaskDefinitions"
    actions   = ["ecs:RegisterTaskDefinition", "ecs:DescribeTaskDefinition"]
    resources = ["*"] # these ECS actions don't support resource-level scoping
  }
  statement {
    sid       = "PassOnlySnipsExecutionRole"
    actions   = ["iam:PassRole"]
    resources = [aws_iam_role.execution.arn]
  }
}

resource "aws_iam_role_policy" "github_deploy" {
  name   = "deploy-snip"
  role   = aws_iam_role.github_deploy.id
  policy = data.aws_iam_policy_document.github_deploy.json
}

output "github_deploy_role_arn" {
  description = "Put this in the workflow's aws-actions/configure-aws-credentials role-to-assume."
  value       = aws_iam_role.github_deploy.arn
}
