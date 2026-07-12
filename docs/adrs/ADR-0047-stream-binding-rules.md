# ADR-0047: Stream Binding Rules

Status: Accepted

An event consumer binding must resolve to a stream satisfied by a producer, imported stream, or external stream. A producer binding does not require any consumer.

Multiple consumers may read one stream. Multiple producers may write one stream only when contracts and keying policy are compatible.
