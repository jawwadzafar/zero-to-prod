variable "region" {
  description = "AWS region to deploy into."
  type        = string
  default     = "eu-west-1"
}

variable "environment" {
  description = "Environment name: staging or production."
  type        = string
  validation {
    condition     = contains(["staging", "production"], var.environment)
    error_message = "environment must be staging or production."
  }
}

variable "vpc_cidr" {
  description = "Address range for the VPC (chapter 12.2)."
  type        = string
  default     = "10.20.0.0/16"
}

variable "snip_image" {
  description = "Image to run, ideally pinned by digest (chapter 9.4)."
  type        = string
  default     = "ghcr.io/jawwadzafar/snip:main"
}

variable "api_count" {
  description = "Number of API tasks."
  type        = number
  default     = 2
}

variable "db_instance_class" {
  description = "RDS instance size."
  type        = string
  default     = "db.t4g.micro"
}

variable "multi_az" {
  description = "Run a standby database in a second zone (production: true)."
  type        = bool
  default     = false
}
