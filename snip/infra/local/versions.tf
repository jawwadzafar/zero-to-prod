# snip's whole stack as Terraform (chapter 12.5) — on your own Docker, free.
# The same ideas (providers, resources, variables, state, plan/apply) are what
# you use for real cloud infrastructure in chapter 12.6.
terraform {
  required_version = ">= 1.9"
  required_providers {
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 3.9"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.7"
    }
  }
}

provider "docker" {} # talks to your local Docker daemon
