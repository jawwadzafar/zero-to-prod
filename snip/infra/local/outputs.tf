output "url" {
  description = "Where snip is listening."
  value       = "http://localhost:${var.api_port}"
}

output "create_key" {
  description = "Command to create an API key."
  value       = "docker exec ${docker_container.api.name} snip keys create me"
}

output "database_password" {
  description = "The generated Postgres password."
  value       = random_password.db.result
  sensitive   = true # hidden in plan/apply output; still stored in the state file
}
