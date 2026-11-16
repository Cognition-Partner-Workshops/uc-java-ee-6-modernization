variable "namePrefix" {
  type = string
}

variable "vpcId" {
  type = string
}

variable "publicSubnetIds" {
  type = list(string)
}

variable "securityGroupId" {
  type = string
}

variable "containerPort" {
  type = number
}

variable "healthCheckPath" {
  type    = string
  default = "/daytrader/health"
}

variable "certificateArn" {
  description = "Empty: HTTP listener on 80. Set: HTTPS on 443 plus an HTTP redirect."
  type        = string
  default     = ""
}

variable "stickinessSeconds" {
  description = "Lifetime of the ALB stickiness cookie."
  type        = number
  default     = 86400
}

variable "deletionProtection" {
  type    = bool
  default = true
}
