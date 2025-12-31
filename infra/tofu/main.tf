terraform {
  required_providers {
    hcloud = {
      source  = "hetznercloud/hcloud"
      version = "~> 1.45"
    }
  }
}

variable "hcloud_token" {
  type      = string
  sensitive = true
}

provider "hcloud" {
  token = var.hcloud_token
}

resource "hcloud_ssh_key" "deploy" {
  name       = "gohome-deploy"
  public_key = file("~/.ssh/id_ed25519.pub")
}

resource "hcloud_server" "gohome" {
  name        = "gohome"
  server_type = "cax11"
  image       = "ubuntu-24.04"
  location    = "fsn1"
  ssh_keys    = [hcloud_ssh_key.deploy.id]

  user_data = file("${path.module}/nixos-infect.sh")
}

output "server_ip" {
  value = hcloud_server.gohome.ipv4_address
}
