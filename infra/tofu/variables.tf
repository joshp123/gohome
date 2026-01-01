variable "hcloud_token" {
  type      = string
  sensitive = true
}

variable "s3_access_key" {
  type      = string
  sensitive = true
}

variable "s3_secret_key" {
  type      = string
  sensitive = true
}

variable "s3_endpoint" {
  type = string
}

variable "s3_region" {
  type    = string
  default = "nbg1"
}

variable "s3_bucket" {
  type    = string
  default = "gohome-oauth"
}
