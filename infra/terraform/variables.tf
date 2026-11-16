variable "awsRegion" {
  description = "AWS region."
  type        = string
  default     = "us-east-1"
}

variable "name" {
  description = "Name prefix for every resource."
  type        = string
  default     = "daytrader"
}

variable "environment" {
  description = "Environment name; part of the SSM parameter and secret paths."
  type        = string
  default     = "dev"
}

variable "vpcCidr" {
  description = "CIDR block of the VPC."
  type        = string
  default     = "10.40.0.0/16"
}

variable "azCount" {
  description = "Number of availability zones (one public and one private subnet each)."
  type        = number
  default     = 2
}

variable "allowedIngressCidrs" {
  description = "CIDR blocks allowed to reach the ALB listener."
  type        = list(string)
  default     = ["0.0.0.0/0"]
}

variable "certificateArn" {
  description = "ACM certificate ARN. Empty: plain HTTP listener on 80. Set: HTTPS on 443 and HTTP redirects to it."
  type        = string
  default     = ""
}

variable "imageTag" {
  description = "Tag of the daytrader-go image in the ECR repository this configuration creates."
  type        = string
  default     = "latest"
}

variable "containerPort" {
  description = "Port the Go binary listens on (LISTEN_ADDR)."
  type        = number
  default     = 8080
}

variable "taskCpu" {
  description = "Fargate task CPU units."
  type        = number
  default     = 512
}

variable "taskMemory" {
  description = "Fargate task memory in MiB."
  type        = number
  default     = 1024
}

variable "desiredCount" {
  description = "Number of tasks. Sessions are in process memory, so more than one relies on ALB stickiness."
  type        = number
  default     = 1
}

variable "logRetentionDays" {
  description = "CloudWatch Logs retention."
  type        = number
  default     = 30
}

variable "orderProcessingMode" {
  description = "ORDER_PROCESSING_MODE: sync (legacy default) or async (SQS consumer, the MDB replacement)."
  type        = string
  default     = "sync"

  validation {
    condition     = contains(["sync", "async"], var.orderProcessingMode)
    error_message = "orderProcessingMode must be sync or async."
  }
}

variable "tradeConfig" {
  description = "Overrides for the TradeConfig variables in daytrader-go/README.md (merged over the defaults)."
  type        = map(string)
  default     = {}
}

variable "databaseName" {
  description = "Aurora PostgreSQL database name."
  type        = string
  default     = "daytrader"
}

variable "databaseUser" {
  description = "Aurora PostgreSQL master user."
  type        = string
  default     = "daytrader"
}

variable "auroraEngineVersion" {
  description = "Aurora PostgreSQL engine version."
  type        = string
  default     = "16.4"
}

variable "auroraMinCapacity" {
  description = "Serverless v2 minimum ACU."
  type        = number
  default     = 0.5
}

variable "auroraMaxCapacity" {
  description = "Serverless v2 maximum ACU."
  type        = number
  default     = 4
}

variable "deletionProtection" {
  description = "Deletion protection on the Aurora cluster and the ALB."
  type        = bool
  default     = true
}
