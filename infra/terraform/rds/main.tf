# Aurora PostgreSQL Serverless v2 in the private subnets, and the Secrets Manager secret that
# holds DATABASE_URL (replaces jdbc/TradeDataSource + the J2C alias TradeDataSourceAuthData).

resource "aws_db_subnet_group" "this" {
  name_prefix = "${var.namePrefix}-"
  subnet_ids  = var.privateSubnetIds
}

# URL-safe on purpose: the password is embedded in DATABASE_URL.
resource "random_password" "master" {
  length  = 32
  special = false
}

resource "aws_rds_cluster" "this" {
  cluster_identifier = "${var.namePrefix}-aurora"
  engine             = "aurora-postgresql"
  engine_mode        = "provisioned"
  engine_version     = var.engineVersion
  database_name      = var.databaseName
  master_username    = var.databaseUser
  master_password    = random_password.master.result

  db_subnet_group_name   = aws_db_subnet_group.this.name
  vpc_security_group_ids = [var.securityGroupId]

  storage_encrypted                   = true
  iam_database_authentication_enabled = true
  backup_retention_period             = var.backupRetentionDays
  copy_tags_to_snapshot               = true
  deletion_protection                 = var.deletionProtection
  skip_final_snapshot                 = false
  final_snapshot_identifier           = "${var.namePrefix}-aurora-final"
  enabled_cloudwatch_logs_exports     = ["postgresql"]

  serverlessv2_scaling_configuration {
    min_capacity = var.minCapacity
    max_capacity = var.maxCapacity
  }
}

resource "aws_rds_cluster_instance" "writer" {
  identifier           = "${var.namePrefix}-aurora-1"
  cluster_identifier   = aws_rds_cluster.this.id
  engine               = aws_rds_cluster.this.engine
  engine_version       = aws_rds_cluster.this.engine_version
  instance_class       = "db.serverless"
  db_subnet_group_name = aws_db_subnet_group.this.name
  publicly_accessible  = false
}

# The Go binary reads one variable, DATABASE_URL, so the secret holds the whole URL.
# The ECS task definition references this secret; the value never appears in the task definition.
resource "aws_secretsmanager_secret" "databaseUrl" {
  name_prefix = "${var.secretPath}/database-url-"
  description = "DATABASE_URL for daytrader-go (pgx)"
}

resource "aws_secretsmanager_secret_version" "databaseUrl" {
  secret_id = aws_secretsmanager_secret.databaseUrl.id
  secret_string = format(
    "postgres://%s:%s@%s:%d/%s?sslmode=require",
    var.databaseUser,
    random_password.master.result,
    aws_rds_cluster.this.endpoint,
    aws_rds_cluster.this.port,
    var.databaseName,
  )
}
