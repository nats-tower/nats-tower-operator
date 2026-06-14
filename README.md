
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

## Pod labels & annotations

The operator generates credentials for pods that carry the following labels:

| Label                                          | Description                                                                                          | Required |
| ---------------------------------------------- | -------------------------------------------------------------------------------------------------- | -------- |
| `nats-tower.com/nats-tower-secret`             | Name of the secret to create/populate with the credentials (`nats.creds`).                          | Yes      |
| `nats-tower.com/nats-tower-account`            | Name of the NATS Tower account the user is created in.                                               | Yes      |
| `nats-tower.com/nats-tower-installation`       | Installation public key. Can be omitted if a default installation is configured on the operator.    | No       |
| `nats-tower.com/nats-tower-credential-type`    | Type of credentials to generate. Currently only `user` is supported (the default).                  | No       |
| `nats-tower.com/nats-tower-role`               | Name of the [user role](https://nats-tower.com/user_roles/) to bind the generated user to.          | No       |

### User roles

To create a user with scoped permissions, set the `nats-tower.com/nats-tower-role`
label to the role name. If the role does not exist yet on NATS Tower, it is created
from the following optional annotations (one subject per line, or comma-separated):

| Annotation                            | Description                                              |
| ------------------------------------- | ------------------------------------------------------- |
| `nats-tower.com/nats-tower-publish`   | Subjects the role is allowed to publish to.             |
| `nats-tower.com/nats-tower-subscribe` | Subjects the role is allowed to subscribe to.           |

Notes:

- If the role already exists, its permissions are managed centrally on NATS Tower
  and the annotations are ignored.
- The role is only applied when the user is first created.
- The publish/subscribe annotations require the role label; setting them without it
  records a `MissingRoleLabel` warning event on the pod and no credentials are generated.
