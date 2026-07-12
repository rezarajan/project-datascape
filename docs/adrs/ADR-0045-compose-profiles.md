# ADR-0045: Compose Profiles

Status: Accepted

The Compose target supports local development and single-host production. Single-host production applies hardening to one host and does not imply distributed high availability.

Rendered services are selected from the graph. Production hardening includes digest-pinned images, externalized secrets, reduced privileges, log rotation, and recovery/verification artifacts where supported.
