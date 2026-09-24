# snip on AWS (chapters 12.2–12.6)

Real, production-shaped infrastructure for snip: a VPC across two availability
zones, an Application Load Balancer, ECS Fargate for the API and worker, RDS
Postgres and ElastiCache Redis, with least-privilege security groups and IAM.

> **Applying this costs real money** (the NAT gateway, load balancer, database
> and cache bill by the hour — roughly a few dollars a day at these sizes).
> CI only runs `terraform fmt`, `init -backend=false` and `validate`. If you
> apply it in your own account, set a budget alert first (chapter 12.1) and run
> `terraform destroy` when you're done (chapter 12.7).

```bash
terraform init -backend-config=envs/staging.backend.hcl   # remote state, chapter 12.6
terraform plan  -var-file=envs/staging.tfvars
terraform apply -var-file=envs/staging.tfvars
terraform destroy -var-file=envs/staging.tfvars
```
