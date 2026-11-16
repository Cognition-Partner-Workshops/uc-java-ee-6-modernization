# ECS Fargate: image repository, order queue, configuration parameters, log group,
# IAM roles, cluster, task definition and service.

locals {
  # Everything non-secret the task reads from SSM Parameter Store.
  parameters = merge(var.tradeConfig, {
    ORDER_PROCESSING_MODE = var.orderProcessingMode
    ORDER_QUEUE_URL       = aws_sqs_queue.orders.url
  })

  # Keys are known at plan time (the queue URL is only a value), so for_each is safe.
  parameterNames = toset(concat(keys(var.tradeConfig), ["ORDER_PROCESSING_MODE", "ORDER_QUEUE_URL"]))
}

resource "aws_ecr_repository" "app" {
  name                 = "${var.namePrefix}-go"
  image_tag_mutability = "IMMUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }
}

# --- Order queue: replaces jms/TradeBrokerQueue + DTBroker3MDB. Used only when ORDER_PROCESSING_MODE=async. ---

resource "aws_sqs_queue" "ordersDeadLetter" {
  name                      = "${var.namePrefix}-orders-dlq"
  sqs_managed_sse_enabled   = true
  message_retention_seconds = 1209600
}

resource "aws_sqs_queue" "orders" {
  name                       = "${var.namePrefix}-orders"
  sqs_managed_sse_enabled    = true
  visibility_timeout_seconds = 60

  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.ordersDeadLetter.arn
    maxReceiveCount     = 5
  })
}

# --- Configuration: TradeConfig in SSM Parameter Store (replaces server.xml / TradeConfig statics). ---

resource "aws_ssm_parameter" "config" {
  for_each = local.parameterNames

  name  = "${var.parameterPath}/${each.value}"
  type  = "String"
  value = local.parameters[each.value]
}

resource "aws_cloudwatch_log_group" "app" {
  name              = "/ecs/${var.namePrefix}"
  retention_in_days = var.logRetentionDays
}

# --- IAM ---

data "aws_iam_policy_document" "assumeByTasks" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

# Execution role: what the ECS agent needs to start the task. Scoped to this
# repository, log group, secret and parameters instead of the broad managed policy.
resource "aws_iam_role" "execution" {
  name_prefix        = "${var.namePrefix}-exec-"
  assume_role_policy = data.aws_iam_policy_document.assumeByTasks.json
}

data "aws_iam_policy_document" "execution" {
  statement {
    sid       = "EcrLogin"
    actions   = ["ecr:GetAuthorizationToken"]
    resources = ["*"] # this action does not support resource-level permissions
  }

  statement {
    sid = "PullImage"
    actions = [
      "ecr:BatchCheckLayerAvailability",
      "ecr:BatchGetImage",
      "ecr:GetDownloadUrlForLayer",
    ]
    resources = [aws_ecr_repository.app.arn]
  }

  statement {
    sid = "WriteLogs"
    actions = [
      "logs:CreateLogStream",
      "logs:PutLogEvents",
    ]
    resources = ["${aws_cloudwatch_log_group.app.arn}:*"]
  }

  statement {
    sid       = "ReadDatabaseUrl"
    actions   = ["secretsmanager:GetSecretValue"]
    resources = [var.databaseUrlSecretArn]
  }

  statement {
    sid       = "ReadConfig"
    actions   = ["ssm:GetParameters"]
    resources = [for parameter in aws_ssm_parameter.config : parameter.arn]
  }
}

resource "aws_iam_role_policy" "execution" {
  name   = "start-task"
  role   = aws_iam_role.execution.id
  policy = data.aws_iam_policy_document.execution.json
}

# Task role: what the Go process itself may call. Only the order queue
# (replaces the J2C alias TradeAdminAuthData).
resource "aws_iam_role" "task" {
  name_prefix        = "${var.namePrefix}-task-"
  assume_role_policy = data.aws_iam_policy_document.assumeByTasks.json
}

data "aws_iam_policy_document" "task" {
  statement {
    sid = "OrderQueue"
    actions = [
      "sqs:SendMessage",
      "sqs:ReceiveMessage",
      "sqs:DeleteMessage",
      "sqs:ChangeMessageVisibility",
      "sqs:GetQueueAttributes",
    ]
    resources = [aws_sqs_queue.orders.arn]
  }
}

resource "aws_iam_role_policy" "task" {
  name   = "order-queue"
  role   = aws_iam_role.task.id
  policy = data.aws_iam_policy_document.task.json
}

# --- Cluster, task definition, service ---

resource "aws_ecs_cluster" "this" {
  name = "${var.namePrefix}-cluster"

  setting {
    name  = "containerInsights"
    value = "enabled"
  }
}

resource "aws_ecs_task_definition" "app" {
  family                   = "${var.namePrefix}-go"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = var.taskCpu
  memory                   = var.taskMemory
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = aws_iam_role.task.arn

  runtime_platform {
    operating_system_family = "LINUX"
    cpu_architecture        = "X86_64"
  }

  container_definitions = jsonencode([
    {
      name                   = var.containerName
      image                  = "${aws_ecr_repository.app.repository_url}:${var.imageTag}"
      essential              = true
      command                = ["serve"]
      user                   = var.containerUser
      readonlyRootFilesystem = true

      linuxParameters = {
        initProcessEnabled = true
      }

      portMappings = [
        {
          containerPort = var.containerPort
          protocol      = "tcp"
        }
      ]

      # DATABASE_URL from Secrets Manager, TradeConfig from SSM Parameter Store.
      # Both are resolved by the ECS agent at task start; no value is stored here.
      secrets = concat(
        [
          {
            name      = "DATABASE_URL"
            valueFrom = var.databaseUrlSecretArn
          }
        ],
        [
          for parameterName in sort(tolist(local.parameterNames)) : {
            name      = parameterName
            valueFrom = aws_ssm_parameter.config[parameterName].arn
          }
        ]
      )

      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.app.name
          "awslogs-region"        = var.awsRegion
          "awslogs-stream-prefix" = var.containerName
        }
      }

      ulimits = [
        {
          name      = "nofile"
          softLimit = 65536
          hardLimit = 65536
        }
      ]
    }
  ])
}

resource "aws_ecs_service" "app" {
  name                              = "${var.namePrefix}-go"
  cluster                           = aws_ecs_cluster.this.id
  task_definition                   = aws_ecs_task_definition.app.arn
  desired_count                     = var.desiredCount
  launch_type                       = "FARGATE"
  platform_version                  = "LATEST"
  health_check_grace_period_seconds = 60
  enable_execute_command            = false

  deployment_minimum_healthy_percent = 100
  deployment_maximum_percent         = 200

  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }

  network_configuration {
    subnets          = var.privateSubnetIds
    security_groups  = [var.securityGroupId]
    assign_public_ip = false
  }

  load_balancer {
    target_group_arn = var.targetGroupArn
    container_name   = var.containerName
    container_port   = var.containerPort
  }
}
