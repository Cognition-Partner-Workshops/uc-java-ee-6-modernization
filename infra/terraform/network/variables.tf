variable "namePrefix" {
  type = string
}

variable "vpcCidr" {
  type = string
}

variable "azCount" {
  type    = number
  default = 2

  validation {
    condition     = var.azCount >= 2
    error_message = "The ALB and the Aurora subnet group need at least two availability zones."
  }
}

variable "containerPort" {
  type = number
}

variable "listenerPort" {
  description = "ALB listener port open to allowedIngressCidrs."
  type        = number
}

variable "redirectHttp" {
  description = "Also open port 80 on the ALB (HTTP to HTTPS redirect)."
  type        = bool
  default     = false
}

variable "allowedIngressCidrs" {
  type = list(string)
}
