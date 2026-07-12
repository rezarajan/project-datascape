# Deterministic Compilation

Deterministic compilation means identical declared inputs produce byte-identical deterministic artifacts and the same bundle digest.

Current release normalizes newlines, sorts resources and files, uses stable modes, and avoids timestamps and random IDs.
