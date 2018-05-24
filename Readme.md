# KStats Frontend

## Configuration

| Name                     | Example                                        | Description                                            |
| ------------------------ | ---------------------------------------------- | ------------------------------------------------------ |
|`KSTATS_DATABASE_TYPE`*   |`postgres`                                      | Database driver (only postgres is supported currently) |
|`KSTATS_DATABASE_URL`*    |`postgresql://kstats:hunter2@localhost/statsbot`| Database URL                                           |
|`KSTATS_REDIS_ENABLED`    |`true`                                          | If Redis should be used as cache                       |
|`KSTATS_REDIS_ADDRESS`*   |`localhost:6379`                                | Redis Address                                          |
|`KSTATS_REDIS_PASSWORD`   |`hunter2`                                       | Redis Password                                         |
