# Pi review extension

The Pi package exposes the review extension and the `timber-review` skill.
Install the package with `pi install /path/to/timber` and ensure `timber` is on
`PATH`. The extension registers `/feedback` and publishes review status.

The `git-review:status`, `git-review:request-status`, and `git-review:ready`
events retain their archived names for compatibility with existing Pi extensions.
The implementation is in `extensions/git-review/`.

Run the extension tests with Node.js 23.6 or later:

```sh
node --test pi/extensions/git-review/tests/*.test.ts
```
