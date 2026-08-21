# Controller: HTTP scan API (/checkv2), stats and metrics used by the
# control plane. Development password only — production deployments mount
# their own worker-controller.inc with a strong password (see
# docs/RUNBOOK.md); this file is part of the dev profile.
password = "rspamd_dev_controller";
secure_ip = null;
