terraform {
  required_version = ">= 1.5.0"
}

resource "terraform_data" "demo" {
  input = "infrasage-demo"
}
