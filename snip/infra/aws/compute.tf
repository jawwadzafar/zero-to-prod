# ---------------------------------------------------------------------------
# snip on ECS Fargate behind an Application Load Balancer (chapter 12.3).
# ---------------------------------------------------------------------------
resource "aws_cloudwatch_log_group" "snip" {
  name              = "/snip/${var.environment}"
  retention_in_days = 14
}

resource "aws_ecs_cluster" "this" {
  name = local.name
}

# The execution role lets ECS pull the image, write logs, and read exactly
# one secret on the task's behalf — nothing else (chapter 12.4).
data "aws_iam_policy_document" "ecs_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "execution" {
  name               = "${local.name}-execution"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume.json
}

resource "aws_iam_role_policy_attachment" "execution_managed" {
  role       = aws_iam_role.execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

data "aws_iam_policy_document" "read_db_secret" {
  statement {
    actions   = ["secretsmanager:GetSecretValue"]
    resources = [aws_secretsmanager_secret.db_url.arn]
  }
}

resource "aws_iam_role_policy" "read_db_secret" {
  name   = "read-db-secret"
  role   = aws_iam_role.execution.id
  policy = data.aws_iam_policy_document.read_db_secret.json
}

locals {
  snip_environment = [
    { name = "SNIP_REDIS_ADDR", value = "${aws_elasticache_cluster.redis.cache_nodes[0].address}:6379" },
    { name = "SNIP_BASE_URL", value = "http://${aws_lb.this.dns_name}" },
  ]
  snip_secrets = [
    { name = "SNIP_DATABASE_URL", valueFrom = aws_secretsmanager_secret.db_url.arn },
  ]
  log_config = {
    logDriver = "awslogs"
    options = {
      awslogs-group         = aws_cloudwatch_log_group.snip.name
      awslogs-region        = var.region
      awslogs-stream-prefix = "snip"
    }
  }
}

resource "aws_ecs_task_definition" "api" {
  family                   = "${local.name}-api"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = 256
  memory                   = 512
  execution_role_arn       = aws_iam_role.execution.arn
  runtime_platform {
    operating_system_family = "LINUX"
    cpu_architecture        = "ARM64" # snip's image is multi-arch (chapter 9.4); ARM is cheaper
  }
  container_definitions = jsonencode([{
    name                   = "api"
    image                  = var.snip_image
    essential              = true
    readonlyRootFilesystem = true
    portMappings           = [{ containerPort = 8080 }]
    environment            = local.snip_environment
    secrets                = local.snip_secrets
    logConfiguration       = local.log_config
  }])
}

resource "aws_ecs_task_definition" "worker" {
  family                   = "${local.name}-worker"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = 256
  memory                   = 512
  execution_role_arn       = aws_iam_role.execution.arn
  runtime_platform {
    operating_system_family = "LINUX"
    cpu_architecture        = "ARM64"
  }
  container_definitions = jsonencode([{
    name                   = "worker"
    image                  = var.snip_image
    essential              = true
    readonlyRootFilesystem = true
    entryPoint             = ["/usr/local/bin/snip-worker"]
    environment            = local.snip_environment
    secrets                = local.snip_secrets
    logConfiguration       = local.log_config
  }])
}

#trivy:ignore:AWS-0053 snip's load balancer is its public front door: internet-facing on purpose
resource "aws_lb" "this" {
  name                       = local.name
  load_balancer_type         = "application"
  drop_invalid_header_fields = true # reject malformed headers (request smuggling defence)
  subnets                    = module.network.public_subnet_ids
  security_groups            = [aws_security_group.alb.id]
}

resource "aws_lb_target_group" "api" {
  name        = "${local.name}-api"
  port        = 8080
  protocol    = "HTTP"
  target_type = "ip"
  vpc_id      = module.network.vpc_id
  health_check {
    path    = "/readyz" # readiness, not liveness (chapter 10.2)
    matcher = "200"
  }
  deregistration_delay = 15 # let in-flight requests finish (chapter 10.2's preStop)
}

# HTTP only, to keep the example free of a domain. In production, add an HTTPS
# listener with an ACM certificate and redirect HTTP to it (chapter 7.1).
#trivy:ignore:AWS-0054 example without a domain; production must add HTTPS with an ACM certificate
resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.this.arn
  port              = 80
  protocol          = "HTTP"
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.api.arn
  }
}

resource "aws_ecs_service" "api" {
  name            = "api"
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.api.arn
  desired_count   = var.api_count
  launch_type     = "FARGATE"
  network_configuration {
    subnets         = module.network.private_subnet_ids # spread across zones
    security_groups = [aws_security_group.app.id]
  }
  load_balancer {
    target_group_arn = aws_lb_target_group.api.arn
    container_name   = "api"
    container_port   = 8080
  }
  deployment_minimum_healthy_percent = 100 # like maxUnavailable: 0 (chapter 10.2)
  deployment_maximum_percent         = 200
  deployment_circuit_breaker {
    enable   = true
    rollback = true # a deployment that never gets healthy rolls itself back
  }
  depends_on = [aws_lb_listener.http]
}

resource "aws_ecs_service" "worker" {
  name            = "worker"
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.worker.arn
  desired_count   = 1
  launch_type     = "FARGATE"
  network_configuration {
    subnets         = module.network.private_subnet_ids
    security_groups = [aws_security_group.app.id]
  }
}
