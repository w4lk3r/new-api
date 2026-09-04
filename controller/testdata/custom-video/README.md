# Supplied Custom Video plugin validation

These two files are byte-for-byte copies of the user-supplied Dola integration
artifacts. They are test inputs, not installed production plugins. The IP address
and placeholder token inside the fixture are compared as strings only; the tests
never contact that address or use a real upstream credential.

| File | SHA-256 |
| --- | --- |
| `custom-video.plugin.js` | `97b1c7be28e3e54179db2c0754e814a44e5e6214809b342c6c983c727e062fc9` |
| `custom-video.fixture.json` | `67250943a530a93fd50b5cbaec227253008488d54423d37d5332b43efd5f840c` |

## Offline validation against the rc.31 host

Host baseline: `v1.0.0-rc.31`, commit
`36dbbf0f77e710455e745048f4a32e8120ad3fd2`.

Run from the repository root:

```sh
go test ./controller -run '^TestCustomVideoPlugin' -count=1 -v
```

The original nine fixture cases pass. The normal integration test uses the
production router, TokenAuth, distribution middleware, polling implementation,
and content proxy against local `httptest` servers and a temporary SQLite DB.
Because successful creation is blocked as described below, this test explicitly
**seeds already-paid task records**. It verifies group/model access rejection,
task ownership, upstream-ID lookup with the channel Key, processing/success/failure
mapping, credentialless video download, and a single normal-path failure refund.
It does not prove that submission persists a task or that refunds recover from
database failures/process crashes. SQLite testing is not a three-database
migration compatibility claim.

## Reproducible submission blockers (unmodified rc.31)

The opt-in probe retains the required successful multipart submission contract.
It is skipped by default, not rewritten to treat the defect as correct behavior:

```sh
RUN_UPSTREAM_VIDEO_SUBMIT_PROBE=1 go test ./controller -run '^TestCustomVideoPluginSubmitProbe$' -count=1 -v
```

Both cases fail on the unmodified host:

- Multipart input: the host builds a multipart body and updates the inbound
  request's Content-Type, but does not copy that generated boundary header to
  the outgoing request. The local upstream rejects it with HTTP 400.
- JSON input: the plugin correctly describes an outbound multipart request,
  but the host unconditionally tries to parse the incoming JSON as multipart.
  Creation returns HTTP 500 with `multipart boundary not found` before calling
  the upstream.

The probes configure a whole-task ModelPrice of USD 0.25 and group multiplier 2.
The real billing session pre-consumes USD 0.50, then refunds it after the failed
submission; the probe waits for both wallet and token quota restoration before
closing its database. Neither the supplied JavaScript nor production Go is
modified to make this validation pass.

## Remaining integration requirements

- The plugin does not declare `usageSchema` or `extractUsage`; the submitted
  `seconds=30` does not automatically become a billing multiplier. The host rejects
  `u("seconds") * price` for this plugin. Use a whole-task price, or implement and
  validate a versioned usage schema before enabling per-second expressions.
- The supplied standard video renderer does not expose the failure reason;
  `/custom/v1/videos/:task_id` does include it.
- The supplied plugin rejects repeated inbound `images[]` fields; multiple images
  must use `image_1` through `image_9`. The outbound two-distinct-image assertion
  remains behind the blocked submit probe and has not passed end-to-end.
- Real upstream TLS, authorization, image acceptance, generation, result URL
  availability, shared-IP polling limits, and upstream refunds are unverified.
  No paid request was made.
