terraform {
  backend "s3" {
    bucket         = "onyxchat-tfstate-prod-001"
    key            = "onyxchat/prod/terraform.tfstate"
    region         = "us-east-1"
    dynamodb_table = "onyxchat-tflock"
    encrypt        = true
  }
}
