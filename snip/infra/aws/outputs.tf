output "url" {
  description = "snip's public address (HTTP)."
  value       = "http://${aws_lb.this.dns_name}"
}

output "database_url_secret" {
  description = "Secrets Manager secret holding the database URL."
  value       = aws_secretsmanager_secret.db_url.name
}

output "redis_auth_secret" {
  description = "Secrets Manager secret holding Redis's AUTH token."
  value       = aws_secretsmanager_secret.redis_auth.name
}

output "log_group" {
  value = aws_cloudwatch_log_group.snip.name
}
