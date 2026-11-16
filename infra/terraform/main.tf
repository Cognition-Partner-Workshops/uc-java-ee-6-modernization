# DayTrader Go + PostgreSQL on ECS Fargate. Thin root wiring four modules.
# Validate only: terraform fmt -check -recursive, terraform init -backend=false, terraform validate.

locals {
  namePrefix = "${var.name}-${var.environment}"

  tags = {
    Application = var.name
    Environment = var.environment
    ManagedBy   = "terraform"
  }

  # TradeConfig variables of daytrader-go/README.md; each becomes an SSM parameter.
  # ORDER_PROCESSING_MODE and ORDER_QUEUE_URL are added by the ecs module.
  tradeConfigDefaults = {
    LISTEN_ADDR                = ":${var.containerPort}"
    CONTEXT_PATH               = "/daytrader"
    REST_PATH                  = "/rest"
    DB_MAX_CONNS               = "20"
    MAX_USERS                  = "15000"
    MAX_QUOTES                 = "10000"
    MAX_HOLDINGS               = "10"
    KEY_BLOCK_SIZE             = "1000"
    PUBLISH_QUOTE_PRICE_CHANGE = "false"
    UPDATE_QUOTE_PRICES        = "true"
    MARKET_SUMMARY_INTERVAL    = "20"
  }
}

module "network" {
  source = "./network"

  namePrefix          = local.namePrefix
  vpcCidr             = var.vpcCidr
  azCount             = var.azCount
  containerPort       = var.containerPort
  listenerPort        = var.certificateArn == "" ? 80 : 443
  redirectHttp        = var.certificateArn != ""
  allowedIngressCidrs = var.allowedIngressCidrs
}

module "alb" {
  source = "./alb"

  namePrefix         = local.namePrefix
  vpcId              = module.network.vpcId
  publicSubnetIds    = module.network.publicSubnetIds
  securityGroupId    = module.network.albSecurityGroupId
  containerPort      = var.containerPort
  healthCheckPath    = "/daytrader/health"
  certificateArn     = var.certificateArn
  deletionProtection = var.deletionProtection
}

module "rds" {
  source = "./rds"

  namePrefix         = local.namePrefix
  secretPath         = "${var.name}/${var.environment}"
  privateSubnetIds   = module.network.privateSubnetIds
  securityGroupId    = module.network.databaseSecurityGroupId
  databaseName       = var.databaseName
  databaseUser       = var.databaseUser
  engineVersion      = var.auroraEngineVersion
  minCapacity        = var.auroraMinCapacity
  maxCapacity        = var.auroraMaxCapacity
  deletionProtection = var.deletionProtection
}

module "ecs" {
  source = "./ecs"

  namePrefix           = local.namePrefix
  parameterPath        = "/${var.name}/${var.environment}"
  awsRegion            = var.awsRegion
  privateSubnetIds     = module.network.privateSubnetIds
  securityGroupId      = module.network.appSecurityGroupId
  targetGroupArn       = module.alb.targetGroupArn
  imageTag             = var.imageTag
  containerPort        = var.containerPort
  taskCpu              = var.taskCpu
  taskMemory           = var.taskMemory
  desiredCount         = var.desiredCount
  logRetentionDays     = var.logRetentionDays
  orderProcessingMode  = var.orderProcessingMode
  tradeConfig          = merge(local.tradeConfigDefaults, var.tradeConfig)
  databaseUrlSecretArn = module.rds.databaseUrlSecretArn

  # The service needs the listener (target group attached to a load balancer) first.
  depends_on = [module.alb]
}
