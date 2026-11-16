variable "namePrefix" {
  type = string
}

variable "secretPath" {
  description = "Secrets Manager name prefix, for example daytrader/dev."
  type        = string
}

variable "privateSubnetIds" {
  type = list(string)
}

variable "securityGroupId" {
  type = string
}

variable "databaseName" {
  type    = string
  default = "daytrader"
}

variable "databaseUser" {
  type    = string
  default = "daytrader"
}

variable "engineVersion" {
  type    = string
  default = "16.4"
}

variable "minCapacity" {
  type    = number
  default = 0.5
}

variable "maxCapacity" {
  type    = number
  default = 4
}

variable "backupRetentionDays" {
  type    = number
  default = 7
}

variable "deletionProtection" {
  type    = bool
  default = true
}
