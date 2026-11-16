output "endpoint" {
  value = aws_rds_cluster.this.endpoint
}

output "port" {
  value = aws_rds_cluster.this.port
}

output "clusterArn" {
  value = aws_rds_cluster.this.arn
}

output "databaseUrlSecretArn" {
  description = "Secret whose value is DATABASE_URL."
  value       = aws_secretsmanager_secret.databaseUrl.arn
}
