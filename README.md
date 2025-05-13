
A kubernetes operator to automatically inject credentials into workloads


## Configuration

| Variable                           | Description                                                      | Required                                   |
| ---------------------------------- | ---------------------------------------------------------------- | ------------------------------------------ |
| NATS_TOWER_CLUSTER_ID              | ID of the cluster to match the ACL in NATS Tower                 | Yes                                        |
| NATS_TOWER_DEFAULT_INSTALLATION    | Default installation public key                                  | No                                         |
| NATS_TOWER_INSTALLATIONS_FILE_PATH | Path to installations YAML file                                  | No (defaults to config/installations.yaml) |
| NATS_TOWER_URL                     | URL of NATS Tower                                                | Yes (defaults to empty)                    |
| NATS_TOWER_API_TOKEN               | Tower API token                                                  | Yes (if NATS_TOWER_API_TOKEN_PATH not set) |
| NATS_TOWER_API_TOKEN_PATH          | Path to file containing Tower API token                          | Yes (if NATS_TOWER_API_TOKEN not set)      |
| NATS_TOWER_RESYNC_INTERVAL         | Resync interval in minutes                                       | No (defaults to 0)                         |
| Resource selectors (optional)      | NATS_TOWER_POD_CONFIG_KIND, NATS_TOWER_POD_CONFIG_SELECTOR, etc. | No                                         |
