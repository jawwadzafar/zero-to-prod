# Who may do what to a snip link (chapter 7.10). Policy as code: versioned,
# reviewed and tested like the rest of snip, and evaluated by OPA.
#
# Input:
#   subject:  {"id", "tenant", "roles": [...], "mfa_age_seconds", "support_ticket"}
#   action:   "read" | "update" | "delete"
#   resource: {"type": "link", "owner", "tenant"}
package snip.links

import rego.v1

default allow := false

# Owners manage their own links.
allow if {
	same_tenant
	input.resource.owner == input.subject.id
	input.action in {"read", "update"}
}

# Tenant roles (RBAC): editors can read and change any link in their tenant,
# viewers can read them.
allow if {
	same_tenant
	some role in input.subject.roles
	input.action in role_actions[role]
}

role_actions := {
	"link-editor": {"read", "update"},
	"link-viewer": {"read"},
}

# Deleting is destructive, so it needs a recent second factor (step-up
# authentication, chapter 7.7), by the owner or a tenant admin.
allow if {
	input.action == "delete"
	same_tenant
	can_delete
	recent_mfa
}

can_delete if input.resource.owner == input.subject.id

can_delete if "tenant-admin" in input.subject.roles

recent_mfa if input.subject.mfa_age_seconds <= 900

# Support staff (ABAC: a decision on attributes, not just roles) may read a
# link in any tenant, but only while working an open support ticket for it,
# and never change anything.
allow if {
	"support" in input.subject.roles
	input.action == "read"
	input.subject.support_ticket.tenant == input.resource.tenant
	input.subject.support_ticket.open
}

same_tenant if input.subject.tenant == input.resource.tenant

# Why a request was refused, for the audit log and for the person asking.
# (Say less to the person than to the log: the log may explain, the user
# should just be told "not allowed".)
reasons contains "different tenant" if {
	not same_tenant
	not allow
}

reasons contains "deleting needs a second factor in the last 15 minutes" if {
	input.action == "delete"
	same_tenant
	can_delete
	not recent_mfa
}

reasons contains "no role grants this action" if {
	same_tenant
	not allow
	not input.action == "delete"
}
