# Pi review extension

The Pi package exposes the review extension and the `timber-review` skill.
Install the package with `pi install /path/to/timber` and ensure `timber` is on
`PATH`.
The extension registers `/feedback` and publishes review status.

The `timber-review:status`, `timber-review:request-status`, and `timber-review:ready` events provide status and coordination for other Pi extensions.
The implementation is in `extensions/timber-review/`.

Run the extension tests with Node.js 23.6 or later:

```sh
node --test pi/extensions/timber-review/tests/*.test.ts
```
