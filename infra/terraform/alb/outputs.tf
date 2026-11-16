output "dnsName" {
  value = aws_lb.this.dns_name
}

output "arn" {
  value = aws_lb.this.arn
}

output "targetGroupArn" {
  value = aws_lb_target_group.app.arn
}

output "listenerArn" {
  value = local.useHttps ? aws_lb_listener.https[0].arn : aws_lb_listener.http[0].arn
}
