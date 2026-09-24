variable "name" {
  description = "Name prefix for the network's resources."
  type        = string
}

variable "cidr" {
  description = "The VPC's address range, e.g. 10.20.0.0/16."
  type        = string
}

variable "az_count" {
  description = "How many availability zones to span."
  type        = number
  default     = 2
}
