package proxy

import "strings"

// These method/path contracts come from internal/app/public_api.go and the
// handlers it registers. Parameters match exactly one path segment. New routes
// require an explicit owner here; no prefix, default owner or backend fallback
// is inferred from an authenticated caller or a project's URL.
type route struct {
	method string
	parts  []string
	owner  string
}

func publicRoutes() []route {
	var routes []route
	for owner, contract := range map[string]string{
		"identity":  identityRoutes,
		"creation":  creationRoutes,
		"media":     mediaRoutes,
		"execution": executionRoutes,
	} {
		for _, line := range strings.Split(contract, "\n") {
			fields := strings.Fields(line)
			if len(fields) == 2 {
				routes = append(routes, route{method: fields[0], parts: strings.Split("/api"+fields[1], "/"), owner: owner})
			}
		}
	}
	return routes
}

// routeOwner uses the full method and path. Multiple matches belonging to
// different owners fail closed even if one has more literal path segments.
func routeOwner(routes []route, method, path string) string {
	parts := strings.Split(path, "/")
	owner := ""
	for _, route := range routes {
		if route.method != method || len(parts) != len(route.parts) {
			continue
		}
		match := true
		for i, part := range route.parts {
			if (strings.HasPrefix(part, ":") && parts[i] != "") || part == parts[i] {
				continue
			}
			match = false
			break
		}
		if match {
			if owner != "" && owner != route.owner {
				return ""
			}
			owner = route.owner
		}
	}
	return owner
}

const identityRoutes = `
GET /announcements/public
GET /notifications
GET /notifications/summary
GET /notifications/display
GET /notifications/display/:id/:revision
POST /notifications/display/ack
POST /notifications/read-all
GET /notifications/:id
POST /notifications/:id/read
GET /platform/notifications
POST /platform/notifications
GET /platform/notifications/:id
PUT /platform/notifications/:id
POST /platform/notifications/:id/publish
PATCH /platform/notifications/:id/enabled
GET /platform/notifications/:id/audit
GET /auth-policy
POST /registrations
POST /registrations/:id/resend
POST /registrations/:id/verify
POST /registrations/:id/complete
POST /sessions
GET /session
POST /session/refresh
POST /session/upgrade
DELETE /sessions/:id
POST /password-resets
POST /password-resets/complete
POST /me/reauthentication
POST /me/password
GET /me
PATCH /me
GET /me/preferences
PATCH /me/preferences
GET /me/sessions
POST /me/sessions/revoke-others
GET /me/security-events
POST /me/email-change
POST /me/email-change/:id/verify
POST /me/email-change/:id/complete
DELETE /me/email-change/:id
POST /me/deletion/preview
POST /me/deletion/challenge
POST /me/deletion
GET /platform/users
GET /platform/users/:id
POST /platform/users/:id/password-reset
GET /platform/users/:id/commands/:request
POST /platform/users/:id/disable
POST /platform/users/:id/delete
POST /platform/users/:id/enable
POST /platform/users/:id/unlock
POST /platform/users/:id/revoke-admin
POST /platform/users/:id/revoke-sessions
POST /platform/users/:id/profile-reset
GET /platform/settings
PATCH /platform/settings
POST /platform/admin-grants
GET /platform/admin-grants
POST /platform/admin-grants/:id/cancel
GET /platform/user-statistics
POST /platform/user-exports
GET /platform/user-exports/:id
GET /platform/user-exports/:id/download
GET /platform/audit-events
POST /platform/account-invitations
GET /platform/account-invitations
POST /platform/account-invitations/:id/resend
POST /platform/account-invitations/:id/revoke
GET /me/authority-proposals
POST /me/authority-proposals/:id/accept
POST /me/authority-proposals/:id/decline
POST /invitations/preview
POST /invitations/accept
POST /invitations/decline
GET /platform/diagnostics/overview
GET /platform/diagnostics/logs
GET /platform/diagnostics/logs/:id
`

const creationRoutes = `
GET /me/spaces
POST /teams/:id/invitations
GET /teams/:id/invitations
POST /teams/:id/invitations/:iid/resend
POST /teams/:id/invitations/:iid/revoke
POST /teams/:id/admin-transfers
GET /teams/:id/admin-transfers
POST /teams/:id/admin-transfers/:pid/cancel
GET /teams/:id/member-statistics
POST /teams/:id/member-exports
GET /teams/:id/member-exports/:eid
GET /teams/:id/member-exports/:eid/download
GET /teams/:id/audit-events
POST /platform/teams/:id/invitations
GET /platform/teams/:id/invitations
POST /platform/teams/:id/invitations/:iid/revoke
POST /platform/teams/:id/admin-transfers
POST /teams
GET /teams/:id
PATCH /teams/:id
GET /teams/:id/members
GET /teams/:id/members/:uid
PATCH /teams/:id/members/:uid/role
POST /teams/:id/members/:uid/suspend
POST /teams/:id/members/:uid/restore
POST /teams/:id/members/:uid/remove
POST /teams/:id/leave
POST /teams/:id/archive
POST /teams/:id/restore
PATCH /teams/:id/settings
GET /teams/:id/avatar
GET /teams/:id/members/:uid/avatar
GET /platform/teams/:id/avatar
GET /platform/teams
GET /platform/teams/:id
PATCH /platform/teams/:id
POST /platform/teams/:id/suspend
POST /platform/teams/:id/restore
GET /projects
POST /projects
GET /projects/:pid
PATCH /projects/:pid
DELETE /projects/:pid
POST /projects/:pid/archive
POST /projects/:pid/unarchive
POST /projects/:pid/restore
POST /independent-canvases
GET /style-presets
POST /creation-context
GET /style-library
GET /style-library/:id
POST /style-library
PATCH /style-library/:id
DELETE /style-library/:id
GET /projects/:pid/budget
GET /projects/:pid/model-defaults
PATCH /projects/:pid/model-defaults
GET /project-creations/:key
POST /projects/:pid/copies
GET /projects/:pid/copies
GET /project-copies/:id
POST /project-copies/:id/cancel
POST /project-copies/:id/retry
POST /project-copies/:id/reconcile
GET /project-folders
POST /project-folders
PATCH /project-folders/:folder_id
DELETE /project-folders/:folder_id
PUT /projects/:pid/folder
GET /projects/:pid/canvases
POST /projects/:pid/canvases
GET /canvases
GET /canvases/:id
PATCH /canvases/:id
DELETE /canvases/:id
GET /canvases/:id/information
GET /canvases/:id/director-recovery/:nid
PUT /canvases/:id/director-recovery/:nid
DELETE /canvases/:id/director-recovery/:nid
GET /canvas-locations/:id
GET /canvases/:id/live
GET /canvases/:id/assistant-sessions
POST /canvases/:id/assistant-sessions
GET /canvases/:id/assistant-sessions/:sid
POST /canvases/:id/assistant-sessions/:sid/turns
GET /canvas-assets/templates
POST /canvas-assets
POST /canvas-assets/commands
GET /settings/prompt-preferences
PUT /settings/prompt-preferences/:operation
POST /projects/:pid/prompt-references/validate
GET /projects/:pid/prompt-uploads/:attempt
POST /projects/:pid/prompt-drafts
PUT /projects/:pid/prompt-drafts/:id
GET /projects/:pid/prompt-drafts/:id
GET /projects/:pid/prompt-drafts
POST /projects/:pid/script-file-imports
GET /projects/:pid/script-file-imports
GET /projects/:pid/script-file-imports/:id
POST /projects/:pid/script-file-imports/:id/cancel
POST /projects/:pid/script-file-imports/:id/retry
POST /projects/:pid/script-file-imports/:id/reconcile
GET /projects/:pid/script-workspace
GET /projects/:pid/script-sources
GET /projects/:pid/script-sources/:lineage
POST /projects/:pid/script-sources
PUT /projects/:pid/script-sources/:lineage
DELETE /projects/:pid/script-sources/:lineage
POST /projects/:pid/script-sources/import
POST /projects/:pid/script-sources/reorder
GET /projects/:pid/script-source-writes
GET /projects/:pid/script-source-writes/:wid
POST /projects/:pid/script-source-writes/:wid/cancel
POST /projects/:pid/script-source-writes/:wid/reconcile
GET /projects/:pid/script-versions
GET /projects/:pid/script-sources/:lineage/history
GET /projects/:pid/script-source-snapshots/:sid
GET /projects/:pid/episodes
POST /projects/:pid/episodes/split/resplit
POST /projects/:pid/episodes/split/confirm
GET /episodes/:eid/source-text
GET /episodes/:eid/structure
GET /episodes/:eid/structure/versions
GET /episodes/:eid/structure/versions/:version
POST /episodes/:eid/structure
POST /episodes/:eid/structure/confirm
GET /projects/:pid/script-versions/:vid/source-text
POST /projects/:pid/script-versions/:vid/adopt
GET /projects/:pid/script-versions/:vid/split-confirmations
GET /projects/:pid/script-versions/:vid/split-confirmations/:cid
GET /projects/:pid/bible/:kind
POST /projects/:pid/bible/:kind
POST /projects/:pid/bible/:kind/adopt-result
GET /projects/:pid/bible/:kind/:id
PUT /projects/:pid/bible/:kind/:id
GET /projects/:pid/bible/:kind/:id/versions
GET /projects/:pid/bible/:kind/:id/versions/:version
POST /projects/:pid/bible/:kind/:id/confirm
DELETE /projects/:pid/bible/:kind/:id
POST /projects/:pid/bible/:kind/:id/restore
POST /projects/:pid/bible/:kind/:id/merge
POST /projects/:pid/bible/:kind/:id/split
POST /projects/:pid/bible/:kind/:id/looks
PUT /projects/:pid/bible/:kind/:id/looks/:look
DELETE /projects/:pid/bible/:kind/:id/looks/:look
POST /projects/:pid/bible/:kind/:id/looks/:look/default
PUT /projects/:pid/bible/:kind/:id/looks/:look/references
PUT /projects/:pid/bible/:kind/:id/voice
DELETE /projects/:pid/bible/:kind/:id/voice
GET /projects/:pid/bible-voices
POST /projects/:pid/bible/:kind/:id/adopt-result
GET /subjects
GET /subject-commands/:key
POST /subjects
GET /subjects/:id
PUT /subjects/:id
DELETE /subjects/:id
PUT /subjects/:id/favorite
POST /subjects/batch
GET /subjects/:id/versions/:version
GET /subjects/:id/versions/:version/media/:binding
POST /subjects/export
GET /subjects/sources
GET /subjects/sources/:item_id
GET /subjects/sources/:item_id/preview
POST /subjects/sources/uploads
GET /subjects/voice-options
POST /subjects/voice-jobs
GET /subjects/voice-jobs/:job_id
GET /subject-voice-commands/:key
GET /projects/:pid/agent/sessions
POST /projects/:pid/agent/sessions
GET /projects/:pid/agent/sessions/:sid
PATCH /projects/:pid/agent/sessions/:sid
POST /projects/:pid/agent/sessions/:sid/runs
GET /projects/:pid/agent/sessions/:sid/runs/:rid
POST /projects/:pid/agent/sessions/:sid/runs/:rid/commands
GET /projects/:pid/agent/sessions/:sid/runs/:rid/events
GET /projects/:pid/agent/sessions/:sid/runs/:rid/commands/current
GET /projects/:pid/agent/sessions/:sid/runs
GET /projects/:pid/agent/sessions/:sid/submissions/current
GET /projects/:pid/agent/sessions/:sid/submissions/:requestId
GET /projects/:pid/agent/sessions/:sid/runs/:rid/messages
GET /projects/:pid/agent/sessions/:sid/runs/:rid/messages/:messageId
GET /projects/:pid/agent/skills
POST /projects/:pid/agent/skills
POST /projects/:pid/agent/skills/:id/versions/:version/enabled
`

const mediaRoutes = `
POST /me/avatar/uploads
GET /me/avatar
POST /admin/provider-logos
GET /provider-logos/:id
GET /projects/:pid/media
GET /projects/:pid/media/:asset_id/preview
GET /projects/:pid/media/:asset_id/original
GET /projects/:pid/media/:asset_id/download
POST /projects/:pid/media/uploads
GET /media/library
GET /media/library/items/:item_id
POST /media/library/commands
GET /media/library/items/:item_id/preview
GET /media/library/items/:item_id/download
POST /media/library/uploads
POST /media/library/video-links
POST /media/library/packages/imports
GET /media/library/packages/imports
GET /media/library/packages/imports/:job_id
POST /media/library/packages/imports/:job_id/reconcile
POST /media/library/packages/imports/:job_id/cancel
GET /media/library/packages/export
POST /media/library/purges
GET /media/library/purges
GET /media/library/purges/:job_id
POST /media/library/purges/:job_id/cancel
POST /media/library/purges/:job_id/reconcile
GET /media/library/storage-usage
POST /media/library/transfers
GET /media/library/transfers
GET /media/library/transfers/:job_id
POST /media/library/transfers/:job_id/cancel
POST /media/library/transfers/:job_id/retry
POST /media/library/transfers/:job_id/reconcile
GET /projects/:pid/voice-library
GET /projects/:pid/system-voices
PUT /projects/:pid/system-voices/:speaker/favorite
POST /projects/:pid/voice-library
PATCH /projects/:pid/voice-library/:vid
POST /projects/:pid/media-depths
GET /projects/:pid/media-depths
GET /media-depths/:job_id
GET /media-depths/:job_id/preview
GET /media-depths/:job_id/download
POST /media-depths/:job_id/review
POST /media-depths/:job_id/reconcile
POST /media-depths/:job_id/cancel
POST /media-depths/:job_id/retry
POST /projects/:pid/media-exports
GET /projects/:pid/media-exports
GET /media-exports/:job_id
GET /media-exports/:job_id/preview
POST /media-exports/:job_id/review
POST /media-exports/:job_id/cancel
POST /media-exports/:job_id/retry
GET /media-exports/:job_id/download
GET /media-exports/:job_id/subtitles
GET /media-export-launch
POST /projects/:pid/media-transcriptions
GET /projects/:pid/media-transcriptions
GET /media-transcriptions/:job_id
GET /media-transcriptions/:job_id/result
GET /media-transcriptions/:job_id/subtitles
POST /media-transcriptions/:job_id/cancel
POST /media-transcriptions/:job_id/retry
`

const executionRoutes = `
POST /platform/billing/scopes/:scopeID/credit-commands
GET /platform/billing/scopes/:scopeID/credit-commands/:commandID
GET /platform/billing/scopes/:scopeID/wallet
GET /billing/wallet
GET /analytics/creation
GET /analytics/business
GET /models
GET /admin/provider-adapters
GET /admin/providers
POST /admin/providers
GET /admin/providers/:id
PATCH /admin/providers/:id
GET /admin/providers/:id/deletion-preview
DELETE /admin/providers/:id
PUT /admin/providers/:id/credentials
POST /admin/providers/:id/credentials/:credential_id/disable
POST /admin/providers/:id/credentials/:credential_id/test
POST /admin/providers/:id/credentials/:credential_id/model-directory
POST /admin/providers/:id/credentials/:credential_id/models/:model_id/test
GET /admin/capabilities
GET /admin/models
POST /admin/models
GET /admin/models/:id
POST /admin/models/:id/versions
POST /admin/models/:id/prices
PATCH /admin/models/:id/status
GET /admin/config-requests/:id
GET /admin/providers/:id/credentials
POST /admin/providers/custom
PATCH /admin/providers/:id/logo
GET /admin/models/:id/releases
POST /admin/models/:id/releases
POST /admin/model-releases/:id/activate
GET /admin/capability-defaults
POST /admin/provider-presets/:id/model-directory
GET /admin/provider-checks/:id
GET /admin/providers/:id/checks
GET /admin/providers/:id/test-models
POST /admin/providers/:id/checks/reachability
POST /admin/provider-checks/:id/query
POST /admin/provider-checks/:id/end-tracking
GET /admin/providers/:id/config
POST /admin/providers/:id/models
GET /admin/models/:id/configuration-check
POST /admin/providers/:id/upgrade-preview
POST /admin/providers/:id/apply-preset
GET /admin/providers/:id/onboarding
POST /admin/providers/:id/onboarding/activate
GET /admin/provider-presets
POST /admin/provider-presets
POST /admin/provider-presets/import-preview
POST /admin/provider-presets/:id/versions
PATCH /admin/provider-presets/:id/state
GET /admin/provider-presets/:id/export
POST /admin/provider-presets/:id/connections
PUT /admin/providers/:id/credentials/:credential_id/pool
GET /admin/model-releases/:id/route-pool
PUT /admin/model-releases/:id/route-pool
GET /projects/:pid/prompt-submissions/current
POST /projects/:pid/prompt-submissions/:id/resolve
POST /admin/model-price-preview
POST /projects/:pid/free-operations
POST /projects/:pid/generation-estimate
POST /projects/:pid/quotes
GET /projects/:pid/operations
GET /operations/:id
POST /operations/:id/confirm
POST /operations/:id/cancel
POST /operations/:id/recover
GET /batches/:id
POST /batches/:id/confirm
POST /batches/:id/cancel
POST /batches/:id/resume
POST /operations/history/publications
GET /works/:work_id
GET /operations/history
POST /operations/history/commands
POST /admin/model-releases/:id/trial-quotes
GET /admin/model-releases/:id/trial-recovery
POST /admin/ai-trials
GET /admin/ai-trials/:id
POST /admin/ai-trials/:id/turns
POST /admin/ai-trials/:id/cancel
POST /admin/ai-trials/:id/recover
GET /admin/ai-trials/:id/artifacts/:sequence
`
