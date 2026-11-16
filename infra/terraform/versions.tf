terraform {
  required_version = ">= 1.6.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.60"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
  }
}

# No backend block on purpose: this configuration is validated only
# (terraform init -backend=false, terraform validate). Never applied.
provider "aws" {
  region = var.awsRegion

  default_tags {
    tags = local.tags
  }
}
