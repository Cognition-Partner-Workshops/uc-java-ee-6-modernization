output "vpcId" {
  value = aws_vpc.this.id
}

output "publicSubnetIds" {
  value = aws_subnet.public[*].id
}

output "privateSubnetIds" {
  value = aws_subnet.private[*].id
}

output "albSecurityGroupId" {
  value = aws_security_group.alb.id
}

output "appSecurityGroupId" {
  value = aws_security_group.app.id
}

output "databaseSecurityGroupId" {
  value = aws_security_group.database.id
}
