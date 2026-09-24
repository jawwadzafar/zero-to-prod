variable "snip_image" {
  description = "The snip image to run."
  type        = string
  default     = "ghcr.io/jawwadzafar/snip:main"
}

variable "api_port" {
  description = "Port on your machine where snip's API is published."
  type        = number
  default     = 8080

  validation {
    condition     = var.api_port >= 1024 && var.api_port <= 65535
    error_message = "Use an unprivileged port between 1024 and 65535."
  }
}

variable "workers" {
  description = "How many click-counting workers to run."
  type        = number
  default     = 1
}

variable "log_level" {
  description = "snip's log level: debug, info, warn or error."
  type        = string
  default     = "info"
}
