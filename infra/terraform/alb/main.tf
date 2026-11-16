# Application Load Balancer in the public subnets, ip targets (Fargate awsvpc).

locals {
  useHttps = var.certificateArn != ""
}

resource "aws_lb" "this" {
  name                       = "${var.namePrefix}-alb"
  load_balancer_type         = "application"
  internal                   = false
  subnets                    = var.publicSubnetIds
  security_groups            = [var.securityGroupId]
  drop_invalid_header_fields = true
  enable_deletion_protection = var.deletionProtection
}

resource "aws_lb_target_group" "app" {
  name_prefix          = "dt-"
  vpc_id               = var.vpcId
  target_type          = "ip"
  protocol             = "HTTP"
  port                 = var.containerPort
  deregistration_delay = 30

  health_check {
    path                = var.healthCheckPath
    protocol            = "HTTP"
    port                = "traffic-port"
    matcher             = "200"
    interval            = 15
    timeout             = 5
    healthy_threshold   = 2
    unhealthy_threshold = 3
  }

  # Sessions are held in process memory (web.Sessions), so a browser must keep
  # hitting the task that created its session.
  stickiness {
    enabled         = true
    type            = "lb_cookie"
    cookie_duration = var.stickinessSeconds
  }

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_lb_listener" "http" {
  count = local.useHttps ? 0 : 1

  load_balancer_arn = aws_lb.this.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.app.arn
  }
}

resource "aws_lb_listener" "https" {
  count = local.useHttps ? 1 : 0

  load_balancer_arn = aws_lb.this.arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = var.certificateArn

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.app.arn
  }
}

resource "aws_lb_listener" "httpRedirect" {
  count = local.useHttps ? 1 : 0

  load_balancer_arn = aws_lb.this.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type = "redirect"

    redirect {
      port        = "443"
      protocol    = "HTTPS"
      status_code = "HTTP_301"
    }
  }
}
