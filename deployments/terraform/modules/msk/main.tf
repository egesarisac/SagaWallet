variable "environment" {
  type = string
}

variable "vpc_id" {
  type = string
}

resource "aws_msk_cluster" "kafka" {
  cluster_name           = "fintech-kafka-${var.environment}"
  kafka_version          = "3.5.1"
  number_of_broker_nodes = 2

  broker_node_group_info {
    instance_type   = "kafka.t3.small"
    client_subnets  = ["subnet-mock1", "subnet-mock2"]
    security_groups = ["sg-mock"]
  }

  tags = {
    Environment = var.environment
    Service     = "EventStreaming"
  }
}
