terraform {
  required_providers {
    hcloud = {
      source  = "hetznercloud/hcloud"
      version = "~> 1.45"
    }
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "hcloud" {
  token = var.hcloud_token
}

provider "aws" {
  access_key                  = var.s3_access_key
  secret_key                  = var.s3_secret_key
  region                      = var.s3_region
  s3_use_path_style           = true
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true
  skip_region_validation      = true

  endpoints {
    s3 = var.s3_endpoint
  }
}

resource "hcloud_ssh_key" "deploy" {
  name       = "gohome-deploy"
  public_key = file("~/.ssh/id_ed25519.pub")
}

resource "hcloud_server" "gohome" {
  name        = "gohome"
  server_type = "cax11"
  image       = "ubuntu-24.04"
  location    = "nbg1"
  ssh_keys    = [hcloud_ssh_key.deploy.id]

  user_data = file("${path.module}/nixos-infect.sh")
}

output "server_ip" {
  value = hcloud_server.gohome.ipv4_address
}

resource "aws_s3_bucket" "oauth" {
  bucket = var.s3_bucket
}

output "oauth_bucket" {
  value = aws_s3_bucket.oauth.bucket
}

output "oauth_endpoint" {
  value = var.s3_endpoint
}
