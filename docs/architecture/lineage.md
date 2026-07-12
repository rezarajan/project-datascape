# Lineage

Lineage is optional unless selected by `LineageBinding` or required by `PlatformPolicy`.

lineage admission is one supported lineage gateway. When selected, Datascape generates lineage admission configuration, namespace policy, producer identity rules, quarantine storage, journal storage, replay scaffolding, and verification checks. Direct OpenLineage-to-backend bindings are valid when policy allows them.

lineage admission does not invent lineage semantics. Producers remain responsible for truthful OpenLineage job, run, input, output, facet, code version, source offset, and snapshot metadata.
