locals {
  name = "snip-${var.environment}"
}

module "network" {
  source = "./modules/network"
  name   = local.name
  cidr   = var.vpc_cidr
}

# ---------------------------------------------------------------------------
# Security groups: who may talk to whom (chapter 12.2). Each allows only the
# traffic snip needs, referencing other groups instead of IP ranges.
# ---------------------------------------------------------------------------
resource "aws_security_group" "alb" {
  name        = "${local.name}-alb"
  description = "Load balancer: HTTP from the internet"
  vpc_id      = module.network.vpc_id
}

resource "aws_vpc_security_group_ingress_rule" "alb_http" {
  security_group_id = aws_security_group.alb.id
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "tcp"
  from_port         = 80
  to_port           = 80
}

resource "aws_vpc_security_group_egress_rule" "alb_to_app" {
  security_group_id            = aws_security_group.alb.id
  referenced_security_group_id = aws_security_group.app.id
  ip_protocol                  = "tcp"
  from_port                    = 8080
  to_port                      = 8080
}

resource "aws_security_group" "app" {
  name        = "${local.name}-app"
  description = "snip API and worker tasks"
  vpc_id      = module.network.vpc_id
}

resource "aws_vpc_security_group_ingress_rule" "app_from_alb" {
  security_group_id            = aws_security_group.app.id
  referenced_security_group_id = aws_security_group.alb.id
  ip_protocol                  = "tcp"
  from_port                    = 8080
  to_port                      = 8080
}

# Outbound from snip's tasks: HTTPS to the internet (image pulls from ghcr.io,
# AWS APIs like Secrets Manager and CloudWatch Logs), plus the database and
# cache — and nothing else. (Chapter 12.6 describes the scanner finding this
# narrowed down.)
#trivy:ignore:AWS-0104 HTTPS egress is required for image pulls and AWS APIs; add VPC endpoints to remove it
resource "aws_vpc_security_group_egress_rule" "app_https_out" {
  security_group_id = aws_security_group.app.id
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "tcp"
  from_port         = 443
  to_port           = 443
}

resource "aws_vpc_security_group_egress_rule" "app_to_db" {
  security_group_id            = aws_security_group.app.id
  referenced_security_group_id = aws_security_group.data.id
  ip_protocol                  = "tcp"
  from_port                    = 5432
  to_port                      = 5432
}

resource "aws_vpc_security_group_egress_rule" "app_to_redis" {
  security_group_id            = aws_security_group.app.id
  referenced_security_group_id = aws_security_group.data.id
  ip_protocol                  = "tcp"
  from_port                    = 6379
  to_port                      = 6379
}

resource "aws_security_group" "data" {
  name        = "${local.name}-data"
  description = "Postgres and Redis: only from snip's tasks"
  vpc_id      = module.network.vpc_id
}

resource "aws_vpc_security_group_ingress_rule" "db_from_app" {
  security_group_id            = aws_security_group.data.id
  referenced_security_group_id = aws_security_group.app.id
  ip_protocol                  = "tcp"
  from_port                    = 5432
  to_port                      = 5432
}

resource "aws_vpc_security_group_ingress_rule" "redis_from_app" {
  security_group_id            = aws_security_group.data.id
  referenced_security_group_id = aws_security_group.app.id
  ip_protocol                  = "tcp"
  from_port                    = 6379
  to_port                      = 6379
}

# ---------------------------------------------------------------------------
# Data: managed Postgres and Redis in private subnets (chapter 12.3).
# ---------------------------------------------------------------------------

# An ephemeral password: generated during apply, sent to AWS through
# write-only arguments, and never stored in the Terraform state (chapter 12.6).
ephemeral "random_password" "db" {
  length  = 32
  special = false
}

resource "aws_db_subnet_group" "this" {
  name       = local.name
  subnet_ids = module.network.private_subnet_ids
}

resource "aws_db_instance" "postgres" {
  identifier                   = local.name
  engine                       = "postgres"
  engine_version               = "16"
  instance_class               = var.db_instance_class
  allocated_storage            = 20
  storage_encrypted            = true
  db_name                      = "snip"
  username                     = "snip"
  password_wo                  = ephemeral.random_password.db.result
  password_wo_version          = 1 # bump to rotate
  db_subnet_group_name         = aws_db_subnet_group.this.name
  vpc_security_group_ids       = [aws_security_group.data.id]
  multi_az                     = var.multi_az
  publicly_accessible          = false
  backup_retention_period      = 7
  deletion_protection          = var.environment == "production"
  skip_final_snapshot          = var.environment != "production"
  final_snapshot_identifier    = "${local.name}-final"
  performance_insights_enabled = true
}

# The full connection string snip reads, stored in Secrets Manager — also
# written write-only, so it never appears in state.
resource "aws_secretsmanager_secret" "db_url" {
  name                    = "${local.name}/database-url"
  recovery_window_in_days = 0
}

resource "aws_secretsmanager_secret_version" "db_url" {
  secret_id                = aws_secretsmanager_secret.db_url.id
  secret_string_wo         = "postgres://snip:${ephemeral.random_password.db.result}@${aws_db_instance.postgres.endpoint}/snip?sslmode=require"
  secret_string_wo_version = 1
}

resource "aws_elasticache_subnet_group" "this" {
  name       = local.name
  subnet_ids = module.network.private_subnet_ids
}

resource "aws_elasticache_cluster" "redis" {
  cluster_id         = local.name
  engine             = "redis"
  node_type          = "cache.t4g.micro"
  num_cache_nodes    = 1
  subnet_group_name  = aws_elasticache_subnet_group.this.name
  security_group_ids = [aws_security_group.data.id]
}
