# Configuration editor UX enhancements

This PR builds on #47 (which must merge first). It does not change the
authentication, proxy network model, YAML storage API, or restart authority.

- A successful Save Changes action displays both a saved-to-disk confirmation
  and a prominent **restart required** notice.
- Known configuration validation failures use fixed, actionable text.
  Arbitrary YAML parser/storage errors are never reflected into the browser.
  Sensitive credentials remain server-side.
- A small vanilla JavaScript enhancement warns before navigating away from
  unsaved changes. The form still works without JavaScript.
- The script is served through the existing authenticated handler with
  Cache-Control: no-store and a restricted script-src 'self' policy.
- The terminal look is retained without any frontend build chain.

Restart Service remains disabled; implementing the restart action requires a
separate review of operating-system privileges and process lifecycle.
