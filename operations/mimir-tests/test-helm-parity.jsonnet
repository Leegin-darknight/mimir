local mimir = import 'mimir/mimir.libsonnet';
local overridesExporter = import 'mimir/overrides-exporter.libsonnet';

mimir + overridesExporter {
  _config+:: {
    namespace: 'default',
    external_url: 'mimir.default.svc.cluster.local',
    storage_backend: 'gcs',

    blocks_storage_bucket_name: 'example-blocks-bucket',
    alertmanager_storage_bucket_name: 'example-alertmanager-bucket',
    ruler_storage_bucket_name: 'example-ruler-bucket',
    alertmanager_enabled: true,
    ruler_enabled: true,
    memberlist_ring_enabled: true,
    unregister_ingesters_on_shutdown: false,
  },
}
