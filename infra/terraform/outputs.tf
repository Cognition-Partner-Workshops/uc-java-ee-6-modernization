output "albDnsName" {
  description = "DNS name of the load balancer; the app is at /daytrader/."
  value       = module.alb.dnsName
}

output "ecrRepositoryUrl" {
  description = "Push the image built from daytrader-go/Dockerfile here."
  value       = module.ecs.ecrRepositoryUrl
}

output "ecsClusterName" {
  value = module.ecs.clusterName
}

output "ecsServiceName" {
  value = module.ecs.serviceName
}

output "orderQueueUrl" {
  description = "SQS queue behind ORDER_QUEUE_URL (replaces jms/TradeBrokerQueue)."
  value       = module.ecs.orderQueueUrl
}

output "databaseEndpoint" {
  value = module.rds.endpoint
}

output "databaseUrlSecretArn" {
  description = "Secrets Manager secret holding DATABASE_URL."
  value       = module.rds.databaseUrlSecretArn
}
