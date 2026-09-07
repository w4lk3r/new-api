# Supplied Custom Video plugin validation

These two files are byte-for-byte copies of the user-supplied Dola integration
artifacts. They are test inputs, not installed production plugins. The IP address
and placeholder token inside the fixture are compared as strings only; the tests
never contact that address or use a real upstream credential.

The admin-installable `custom-video` plugin is maintained separately at
[`docs/task-plugins/custom-video`](../../../docs/task-plugins/custom-video/README.md).
Its current version is `1.0.1` and it adds a validated `calls=1` usage fact for
fixed per-request billing. Do not replace the immutable test fixture below with
that production plugin.

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

The original nine fixture cases pass. The integration tests use the
production router, TokenAuth, distribution middleware, polling implementation,
and content proxy against local `httptest` servers and a temporary SQLite DB.
`TestCustomVideoPluginSubmitLifecycle` creates real tasks through this local
pipeline for JSON text-only input and multipart input with zero, one, two, or
three distinct reference files. It verifies upstream form fields/file bytes,
channel authorization, group/model access rejection, persistence and task
ownership, upstream-ID lookup, processing/success/failure mapping, credentialless
video download, and a single normal-path failure refund. Each submitted task
charges the test wallet once; queries and downloads do not charge it again.

`TestCustomVideoPluginSeededTaskLifecycle` separately checks existing already-paid
task records without resubmission. Neither test proves refund recovery from
database failures/process crashes. SQLite testing is not a three-database
migration compatibility claim. This HTTP encoding fix does not change database
models, migrations, drivers, queries, or accounting implementation.

## Multipart regression and fix

On unmodified rc.31, the previously opt-in `TestCustomVideoPluginSubmitProbe`
reproduced two submission blockers. Its successful-submit assertions now run
unconditionally in the expanded lifecycle test:

```sh
go test ./controller -run '^TestCustomVideoPluginSubmitLifecycle$' -count=1 -v
```

Both cases fail on the unmodified host:

- Multipart input: the host builds a multipart body and updates the inbound
  request's Content-Type, but does not copy that generated boundary header to
  the outgoing request. The local upstream rejects it with HTTP 400.
- JSON input: the plugin correctly describes an outbound multipart request,
  but the host unconditionally tries to parse the incoming JSON as multipart.
  Creation returns HTTP 500 with `multipart boundary not found` before calling
  the upstream.

The fix retains the encoder-generated Content-Type on the adaptor and applies
it to the actual outgoing request after plugin headers. The client's original
Content-Type is unchanged. JSON input can produce a text-only multipart body;
malformed multipart input and nonexistent file references remain rejected.

Tests configure a whole-task ModelPrice of USD 0.25 and group multiplier 2.
The real billing session pre-consumes USD 0.50 in the temporary test database.
Failed upstream tasks refund both wallet and token quota exactly once under
repeated polling. The supplied JavaScript and fixture files remain unchanged.

## Remaining integration requirements

- The plugin does not declare `usageSchema` or `extractUsage`; the submitted
  `seconds=30` does not automatically become a billing multiplier. The host rejects
  `u("seconds") * price` for this plugin. Use a whole-task price, or implement and
  validate a versioned usage schema before enabling per-second expressions.
- The supplied standard video renderer does not expose the failure reason;
  `/custom/v1/videos/:task_id` does include it.
- The supplied plugin rejects repeated inbound `images[]` fields; multiple images
  must use `image_1` through `image_9`. Local HTTP tests now verify the outgoing
  repeated `images[]` fields preserve up to three distinct filenames and bodies.
- Real upstream TLS, authorization, image acceptance, generation, result URL
  availability, shared-IP polling limits, and upstream refunds are unverified.
  No paid request was made.
