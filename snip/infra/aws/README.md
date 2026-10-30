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

## Two database logins (chapter 7.4)

RDS's master user, `snip`, owns the schema. Its connection string lives in the
`…/database-owner-url` secret and goes to one place only: the `migrate`
container in the API's task definition, which runs `snip migrate` and exits
before the `api` container starts (`dependsOn … SUCCESS`, ECS's init
container). People use the master user to break glass, not day to day.

The API and the worker sign in as `snip_app`, which can read and write rows but
can't change the schema or the audit trail. Terraform generates its password
into the `…/database-url` secret but can't create the login itself: the
database sits in private subnets that Terraform, running on your laptop or in
CI, can't reach. The `migrate` container creates the login (or resets its
password) from that secret every time an API task starts.

If you applied this configuration before the `snip_app` login existed, bump
`password_wo_version` and both database secrets' `secret_string_wo_version` to
2 in one apply: that resets the master password and writes both connection
strings from the same apply's passwords.
