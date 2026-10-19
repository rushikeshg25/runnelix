# GitHub: create issue

Node: `github.createIssue@1`. Effect: unsafe external write.

Configuration: `owner`, `repository`, and a granted `credentialRef`. The credential destination must be `api.github.com`. Use a fine-grained token with Issues write access limited to the intended repository. Approve that hostname in server egress settings.

Input: `{ "title": "Fixture example", "body": "Optional description" }`. The title is limited to 256 characters and the body to 6,000. Output: `{ "id": 123, "number": 7, "url": "https://github.com/example/project/issues/7" }`.

The adapter pins the GitHub REST API header to `2022-11-28`. It sends one POST and retains only issue identifiers. The shared network boundary injects the token, checks the exact destination, rejects redirects, and bounds the response.

A lost or malformed response can follow a successful creation. Check the repository before resolving an uncertain invocation. Confirm only the real issue identifiers. Do not start another run to find out whether the first issue was created.

Verification uses fake responses. Live-provider permissions and behavior remain unverified. See [provider contracts](provider-contracts.md).
