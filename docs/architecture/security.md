# Security

Generated bundles must not contain secret values. Specifications should reference secrets instead of embedding them.

The compiler rejects inline secret-like fields such as password, secret, token, or private key values unless they look like references.
