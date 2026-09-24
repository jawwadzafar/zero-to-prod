# Remote state for production (chapter 12.6). Create the bucket once, by hand or
# with a small bootstrap configuration; use a name that is unique to you.
bucket       = "YOUR-UNIQUE-BUCKET-snip-tfstate"
key          = "snip/production/terraform.tfstate"
region       = "eu-west-1"
encrypt      = true
use_lockfile = true # S3-native state locking
