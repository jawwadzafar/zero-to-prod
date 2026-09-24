terraform {
  required_version = ">= 1.9"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.7"
    }
  }
  # Remote state in S3 with locking (chapter 12.6). The bucket and key come
  # from envs/<env>.backend.hcl, so one configuration serves every environment.
  backend "s3" {}
}

provider "aws" {
  region = var.region
  default_tags {
    tags = {
      project     = "snip"
      environment = var.environment
      managed-by  = "terraform"
    }
  }
}
