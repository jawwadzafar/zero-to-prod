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
  engine_version               = "18"
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

# Two logins, two connection strings in Secrets Manager, both written
# write-only so they never appear in state (chapter 7.4):
#  - the master user, snip, owns the schema. Only the migrate container (in
#    compute.tf) gets it; people use it to break glass, never day to day.
#  - snip_app is what the API and worker sign in as. Terraform can't reach a
#    database in a private subnet, so it doesn't create this login: it only
#    generates the password. The migrate container creates the login with it.
resource "aws_secretsmanager_secret" "db_owner_url" {
  name                    = "${local.name}/database-owner-url"
  recovery_window_in_days = 0
}

resource "aws_secretsmanager_secret_version" "db_owner_url" {
  secret_id                = aws_secretsmanager_secret.db_owner_url.id
  secret_string_wo         = "postgres://snip:${ephemeral.random_password.db.result}@${aws_db_instance.postgres.endpoint}/snip?sslmode=require"
  secret_string_wo_version = 1 # bump together with password_wo_version
}

ephemeral "random_password" "db_app" {
  length  = 32
  special = false
}

resource "aws_secretsmanager_secret" "db_url" {
  name                    = "${local.name}/database-url"
  recovery_window_in_days = 0
}

resource "aws_secretsmanager_secret_version" "db_url" {
  secret_id                = aws_secretsmanager_secret.db_url.id
  secret_string_wo         = "postgres://snip_app:${ephemeral.random_password.db_app.result}@${aws_db_instance.postgres.endpoint}/snip?sslmode=require"
  secret_string_wo_version = 1 # bump to rotate; the next migrate container applies it
}

resource "aws_elasticache_subnet_group" "this" {
  name       = local.name
  subnet_ids = module.network.private_subnet_ids
}

# Redis's AUTH token, generated and stored the same way as the database
# password: ephemeral, write-only, never in state (chapter 7.6).
ephemeral "random_password" "redis" {
  length  = 32
  special = false # ElastiCache rejects some symbols in tokens; length makes it strong
}

# A replication group (one node) rather than a bare cluster, because only a
# replication group can require a token and encrypt traffic in transit.
resource "aws_elasticache_replication_group" "redis" {
  replication_group_id       = local.name
  description                = "snip cache, rate limits and click queue"
  engine                     = "redis"
  node_type                  = "cache.t4g.micro"
  num_cache_clusters         = 1
  subnet_group_name          = aws_elasticache_subnet_group.this.name
  security_group_ids         = [aws_security_group.data.id]
  transit_encryption_enabled = true # TLS only: snip sets SNIP_REDIS_TLS=true (compute.tf)
  at_rest_encryption_enabled = true
  auth_token_wo              = ephemeral.random_password.redis.result
  auth_token_wo_version      = 1 # bump to rotate
}

resource "aws_secretsmanager_secret" "redis_auth" {
  name                    = "${local.name}/redis-auth-token"
  recovery_window_in_days = 0
}

resource "aws_secretsmanager_secret_version" "redis_auth" {
  secret_id                = aws_secretsmanager_secret.redis_auth.id
  secret_string_wo         = ephemeral.random_password.redis.result
  secret_string_wo_version = 1
}
