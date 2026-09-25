package snip.links_test

import rego.v1

import data.snip.links

ada := {"id": "ada", "tenant": "acme", "roles": [], "mfa_age_seconds": 99999}

link := {"type": "link", "owner": "ada", "tenant": "acme"}

test_owner_reads_and_updates_own_link if {
	links.allow with input as {"subject": ada, "action": "read", "resource": link}
	links.allow with input as {"subject": ada, "action": "update", "resource": link}
}

test_owner_cannot_touch_another_tenants_link if {
	other := object.union(link, {"tenant": "globex"})
	not links.allow with input as {"subject": ada, "action": "read", "resource": other}
	"different tenant" in links.reasons with input as {"subject": ada, "action": "read", "resource": other}
}

test_viewer_reads_but_cannot_update if {
	grace := {"id": "grace", "tenant": "acme", "roles": ["link-viewer"]}
	bobs := object.union(link, {"owner": "bob"})
	links.allow with input as {"subject": grace, "action": "read", "resource": bobs}
	not links.allow with input as {"subject": grace, "action": "update", "resource": bobs}
	"no role grants this action" in links.reasons with input as {"subject": grace, "action": "update", "resource": bobs}
}

test_delete_needs_recent_mfa if {
	not links.allow with input as {"subject": ada, "action": "delete", "resource": link}
	"deleting needs a second factor in the last 15 minutes" in links.reasons with input as {"subject": ada, "action": "delete", "resource": link}

	fresh := object.union(ada, {"mfa_age_seconds": 120})
	links.allow with input as {"subject": fresh, "action": "delete", "resource": link}
}

test_editor_cannot_delete_others_links if {
	ed := {"id": "ed", "tenant": "acme", "roles": ["link-editor"], "mfa_age_seconds": 10}
	not links.allow with input as {"subject": ed, "action": "delete", "resource": link}
}

test_tenant_admin_deletes_with_mfa if {
	admin := {"id": "root", "tenant": "acme", "roles": ["tenant-admin"], "mfa_age_seconds": 10}
	links.allow with input as {"subject": admin, "action": "delete", "resource": link}
}

test_support_reads_only_with_an_open_ticket_for_that_tenant if {
	base := {"id": "sam", "tenant": "snip-staff", "roles": ["support"]}
	with_ticket := object.union(base, {"support_ticket": {"tenant": "acme", "open": true}})
	links.allow with input as {"subject": with_ticket, "action": "read", "resource": link}
	not links.allow with input as {"subject": with_ticket, "action": "update", "resource": link}
	not links.allow with input as {"subject": base, "action": "read", "resource": link}

	closed := object.union(base, {"support_ticket": {"tenant": "acme", "open": false}})
	not links.allow with input as {"subject": closed, "action": "read", "resource": link}

	wrong := object.union(base, {"support_ticket": {"tenant": "globex", "open": true}})
	not links.allow with input as {"subject": wrong, "action": "read", "resource": link}
}

test_unknown_action_is_denied if {
	not links.allow with input as {"subject": ada, "action": "export", "resource": link}
}
