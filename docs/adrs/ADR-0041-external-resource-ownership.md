# ADR-0041: External Resource Ownership

Status: Accepted

Resources can be `managed`, `external`, `imported`, `planned`, or `disabled`. External and imported resources can satisfy graph dependencies when they declare capability, interface, trust, and verification metadata.

Datascape renders verification, connection metadata, documentation, and optional projections for external resources. It does not render containers or services for externally owned components.
