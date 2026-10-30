# A private network, like Compose's default network (chapter 9.3).
resource "docker_network" "snip" {
  name = "snip-tf"
}

# A generated database password: never typed, never committed.
# (It is stored in the Terraform state, which must be protected — chapter 12.6.)
resource "random_password" "db" {
  length  = 24
  special = false
}

# snip_app, the login the API and the worker use, gets its own: it can read and
# write rows but not change the schema or the audit trail (chapter 7.4). The
# first password is the schema owner's, used only by the migrate container.
resource "random_password" "db_app" {
  length  = 24
  special = false
}

# Redis gets one too, so only snip's containers can read or rewrite the cache
# and the click queue (chapter 7.6).
resource "random_password" "redis" {
  length  = 24
  special = false
}

resource "docker_volume" "pgdata" {
  name = "snip-tf-pgdata"
}

resource "docker_image" "postgres" {
  name         = "postgres:18-alpine"
  keep_locally = true
}

resource "docker_image" "redis" {
  name         = "redis:7-alpine"
  keep_locally = true
}

resource "docker_image" "snip" {
  name         = var.snip_image
  keep_locally = true
}

resource "docker_container" "postgres" {
  name  = "snip-tf-postgres"
  image = docker_image.postgres.image_id
  env = [
    "POSTGRES_USER=snip",
    "POSTGRES_PASSWORD=${random_password.db.result}",
    "POSTGRES_DB=snip",
  ]
  networks_advanced {
    name = docker_network.snip.id
  }
  volumes {
    volume_name    = docker_volume.pgdata.name
    container_path = "/var/lib/postgresql"
  }
  healthcheck {
    test     = ["CMD-SHELL", "pg_isready -U snip"]
    interval = "3s"
    retries  = 20
  }
  wait = true # don't report success until the healthcheck passes
}

resource "docker_container" "redis" {
  name    = "snip-tf-redis"
  image   = docker_image.redis.image_id
  command = ["redis-server", "--requirepass", random_password.redis.result]
  env     = ["REDISCLI_AUTH=${random_password.redis.result}"] # lets redis-cli in the container log in
  networks_advanced {
    name = docker_network.snip.id
  }
  healthcheck {
    test     = ["CMD", "redis-cli", "ping"]
    interval = "3s"
    retries  = 20
  }
  wait = true
}

locals {
  db_address = "${docker_container.postgres.name}:5432/snip?sslmode=disable"
  snip_env = [
    "SNIP_DATABASE_URL=postgres://snip_app:${random_password.db_app.result}@${local.db_address}",
    "SNIP_REDIS_ADDR=${docker_container.redis.name}:6379",
    "SNIP_REDIS_PASSWORD=${random_password.redis.result}",
    "SNIP_BASE_URL=http://localhost:${var.api_port}",
    "SNIP_LOG_LEVEL=${var.log_level}",
  ]
}

# A one-off container: `snip migrate` as the schema's owner (Postgres's
# superuser here), which also creates snip_app's login from SNIP_DATABASE_URL.
# Terraform waits for it to exit, and apply fails unless it exited cleanly.
resource "docker_container" "migrate" {
  name     = "snip-tf-migrate"
  image    = docker_image.snip.image_id
  command  = ["migrate"]
  env      = concat(local.snip_env, ["SNIP_MIGRATE_DATABASE_URL=postgres://snip:${random_password.db.result}@${local.db_address}"])
  attach   = true  # wait until it exits
  must_run = false # exited is its finished state, not drift
  networks_advanced {
    name = docker_network.snip.id
  }
  lifecycle {
    postcondition {
      condition     = self.exit_code == 0
      error_message = "snip migrate failed: see docker logs snip-tf-migrate."
    }
  }
}

resource "docker_container" "api" {
  name  = "snip-tf-api"
  image = docker_image.snip.image_id
  env   = local.snip_env
  networks_advanced {
    name = docker_network.snip.id
  }
  ports {
    internal = 8080
    external = var.api_port
  }
  depends_on = [docker_container.migrate] # the schema and snip_app's login exist first
}

resource "docker_container" "worker" {
  count      = var.workers # one resource block, N containers
  name       = "snip-tf-worker-${count.index}"
  image      = docker_image.snip.image_id
  entrypoint = ["/usr/local/bin/snip-worker"]
  env        = local.snip_env
  networks_advanced {
    name = docker_network.snip.id
  }
  depends_on = [docker_container.migrate] # the schema and snip_app's login exist first
}
