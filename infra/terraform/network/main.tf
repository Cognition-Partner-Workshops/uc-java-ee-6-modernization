# VPC: public subnets (ALB, NAT gateway) and private subnets (tasks, database), plus the three
# security groups. The groups live here so alb -> app -> database can reference each other
# without a module cycle.

data "aws_availability_zones" "available" {
  state = "available"
}

locals {
  azNames = slice(data.aws_availability_zones.available.names, 0, var.azCount)
}

resource "aws_vpc" "this" {
  cidr_block           = var.vpcCidr
  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = { Name = "${var.namePrefix}-vpc" }
}

resource "aws_internet_gateway" "this" {
  vpc_id = aws_vpc.this.id

  tags = { Name = "${var.namePrefix}-igw" }
}

resource "aws_subnet" "public" {
  count = var.azCount

  vpc_id                  = aws_vpc.this.id
  availability_zone       = local.azNames[count.index]
  cidr_block              = cidrsubnet(var.vpcCidr, 4, count.index)
  map_public_ip_on_launch = false

  tags = { Name = "${var.namePrefix}-public-${local.azNames[count.index]}", Tier = "public" }
}

resource "aws_subnet" "private" {
  count = var.azCount

  vpc_id            = aws_vpc.this.id
  availability_zone = local.azNames[count.index]
  cidr_block        = cidrsubnet(var.vpcCidr, 4, count.index + 8)

  tags = { Name = "${var.namePrefix}-private-${local.azNames[count.index]}", Tier = "private" }
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.this.id

  tags = { Name = "${var.namePrefix}-public" }
}

resource "aws_route" "publicInternet" {
  route_table_id         = aws_route_table.public.id
  destination_cidr_block = "0.0.0.0/0"
  gateway_id             = aws_internet_gateway.this.id
}

resource "aws_route_table_association" "public" {
  count = var.azCount

  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public.id
}

# One NAT gateway: tasks in the private subnets pull the image and reach
# Secrets Manager, SSM, SQS and CloudWatch Logs through it.
resource "aws_eip" "nat" {
  domain = "vpc"

  tags = { Name = "${var.namePrefix}-nat" }
}

resource "aws_nat_gateway" "this" {
  allocation_id = aws_eip.nat.id
  subnet_id     = aws_subnet.public[0].id

  tags = { Name = "${var.namePrefix}-nat" }

  depends_on = [aws_internet_gateway.this]
}

resource "aws_route_table" "private" {
  vpc_id = aws_vpc.this.id

  tags = { Name = "${var.namePrefix}-private" }
}

resource "aws_route" "privateNat" {
  route_table_id         = aws_route_table.private.id
  destination_cidr_block = "0.0.0.0/0"
  nat_gateway_id         = aws_nat_gateway.this.id
}

resource "aws_route_table_association" "private" {
  count = var.azCount

  subnet_id      = aws_subnet.private[count.index].id
  route_table_id = aws_route_table.private.id
}

# Lock the default security group: nothing uses it.
resource "aws_default_security_group" "this" {
  vpc_id = aws_vpc.this.id
}

# --- Security groups: internet -> alb -> app -> database, nothing else. ---

resource "aws_security_group" "alb" {
  name_prefix = "${var.namePrefix}-alb-"
  description = "DayTrader ALB"
  vpc_id      = aws_vpc.this.id

  tags = { Name = "${var.namePrefix}-alb" }

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_security_group" "app" {
  name_prefix = "${var.namePrefix}-app-"
  description = "DayTrader Fargate tasks"
  vpc_id      = aws_vpc.this.id

  tags = { Name = "${var.namePrefix}-app" }

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_security_group" "database" {
  name_prefix = "${var.namePrefix}-db-"
  description = "DayTrader Aurora PostgreSQL"
  vpc_id      = aws_vpc.this.id

  tags = { Name = "${var.namePrefix}-db" }

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_vpc_security_group_ingress_rule" "albListener" {
  for_each = toset(var.allowedIngressCidrs)

  security_group_id = aws_security_group.alb.id
  description       = "Listener from ${each.value}"
  cidr_ipv4         = each.value
  ip_protocol       = "tcp"
  from_port         = var.listenerPort
  to_port           = var.listenerPort
}

resource "aws_vpc_security_group_ingress_rule" "albRedirect" {
  for_each = var.redirectHttp ? toset(var.allowedIngressCidrs) : toset([])

  security_group_id = aws_security_group.alb.id
  description       = "HTTP redirect from ${each.value}"
  cidr_ipv4         = each.value
  ip_protocol       = "tcp"
  from_port         = 80
  to_port           = 80
}

resource "aws_vpc_security_group_egress_rule" "albToApp" {
  security_group_id            = aws_security_group.alb.id
  description                  = "Forward and health check to the tasks"
  referenced_security_group_id = aws_security_group.app.id
  ip_protocol                  = "tcp"
  from_port                    = var.containerPort
  to_port                      = var.containerPort
}

resource "aws_vpc_security_group_ingress_rule" "appFromAlb" {
  security_group_id            = aws_security_group.app.id
  description                  = "HTTP from the ALB only"
  referenced_security_group_id = aws_security_group.alb.id
  ip_protocol                  = "tcp"
  from_port                    = var.containerPort
  to_port                      = var.containerPort
}

resource "aws_vpc_security_group_egress_rule" "appToDatabase" {
  security_group_id            = aws_security_group.app.id
  description                  = "PostgreSQL"
  referenced_security_group_id = aws_security_group.database.id
  ip_protocol                  = "tcp"
  from_port                    = 5432
  to_port                      = 5432
}

resource "aws_vpc_security_group_egress_rule" "appToAwsApis" {
  security_group_id = aws_security_group.app.id
  description       = "HTTPS to ECR, Secrets Manager, SSM, SQS and CloudWatch Logs through the NAT gateway"
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "tcp"
  from_port         = 443
  to_port           = 443
}

# The database group has no egress rule at all.
resource "aws_vpc_security_group_ingress_rule" "databaseFromApp" {
  security_group_id            = aws_security_group.database.id
  description                  = "PostgreSQL from the tasks only"
  referenced_security_group_id = aws_security_group.app.id
  ip_protocol                  = "tcp"
  from_port                    = 5432
  to_port                      = 5432
}
