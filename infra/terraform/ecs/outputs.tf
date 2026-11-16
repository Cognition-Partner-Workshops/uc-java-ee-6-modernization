output "clusterName" {
  value = aws_ecs_cluster.this.name
}

output "serviceName" {
  value = aws_ecs_service.app.name
}

output "taskDefinitionArn" {
  value = aws_ecs_task_definition.app.arn
}

output "ecrRepositoryUrl" {
  value = aws_ecr_repository.app.repository_url
}

output "orderQueueUrl" {
  value = aws_sqs_queue.orders.url
}

output "orderQueueArn" {
  value = aws_sqs_queue.orders.arn
}

output "executionRoleArn" {
  value = aws_iam_role.execution.arn
}

output "taskRoleArn" {
  value = aws_iam_role.task.arn
}

output "logGroupName" {
  value = aws_cloudwatch_log_group.app.name
}
