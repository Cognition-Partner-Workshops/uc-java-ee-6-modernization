variable "namePrefix" {
  type = string
}

variable "parameterPath" {
  description = "SSM Parameter Store path, for example /daytrader/dev."
  type        = string
}

variable "awsRegion" {
  type = string
}

variable "privateSubnetIds" {
  type = list(string)
}

variable "securityGroupId" {
  type = string
}

variable "targetGroupArn" {
  type = string
}

variable "imageTag" {
  type    = string
  default = "latest"
}

variable "containerName" {
  type    = string
  default = "daytrader"
}

variable "containerPort" {
  type    = number
  default = 8080
}

variable "containerUser" {
  description = "uid:gid of the image (distroless nonroot)."
  type        = string
  default     = "65532:65532"
}

variable "taskCpu" {
  type    = number
  default = 512
}

variable "taskMemory" {
  type    = number
  default = 1024
}

variable "desiredCount" {
  type    = number
  default = 1
}

variable "logRetentionDays" {
  type    = number
  default = 30
}

variable "orderProcessingMode" {
  description = "sync (default, what the parity baseline uses) or async (SQS consumer)."
  type        = string
  default     = "sync"
}

variable "tradeConfig" {
  description = "Non-secret environment variables (TradeConfig); one SSM parameter each."
  type        = map(string)
}

variable "databaseUrlSecretArn" {
  description = "Secrets Manager secret whose value is DATABASE_URL."
  type        = string
}
