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

resource "docker_volume" "pgdata" {
  name = "snip-tf-pgdata"
}

resource "docker_image" "postgres" {
  name         = "postgres:16-alpine"
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
    container_path = "/var/lib/postgresql/data"
  }
  healthcheck {
    test     = ["CMD-SHELL", "pg_isready -U snip"]
    interval = "3s"
    retries  = 20
  }
  wait = true # don't report success until the healthcheck passes
}

resource "docker_container" "redis" {
  name  = "snip-tf-redis"
  image = docker_image.redis.image_id
  networks_advanced {
    name = docker_network.snip.id
  }
}

locals {
  snip_env = [
    "SNIP_DATABASE_URL=postgres://snip:${random_password.db.result}@${docker_container.postgres.name}:5432/snip?sslmode=disable",
    "SNIP_REDIS_ADDR=${docker_container.redis.name}:6379",
    "SNIP_BASE_URL=http://localhost:${var.api_port}",
    "SNIP_LOG_LEVEL=${var.log_level}",
  ]
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
  depends_on = [docker_container.api] # the API runs the migrations first
}
