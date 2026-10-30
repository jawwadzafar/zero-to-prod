output "url" {
  description = "Where snip is listening."
  value       = "http://localhost:${var.api_port}"
}

output "create_key" {
  description = "Command to create an API key."
  value       = "docker exec ${docker_container.api.name} snip keys create me"
}

output "database_password" {
  description = "The generated password of Postgres's owner login, snip."
  value       = random_password.db.result
  sensitive   = true # hidden in plan/apply output; still stored in the state file
}

output "app_database_password" {
  description = "The generated password of snip_app, the login the API and worker use."
  value       = random_password.db_app.result
  sensitive   = true
}

output "redis_password" {
  description = "The generated Redis password."
  value       = random_password.redis.result
  sensitive   = true
}
