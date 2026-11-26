# Opt-in restart for Sparky (Linux and macOS)

The authenticated `sparky-config` web UI edits the YAML configuration but does
not restart the running gateway when saving. This optional feature makes
**Restart Sparky** available to the administrator. It is disabled by default.

The web configuration process **must remain separate** from the main Sparky
service. A service-manager restart can then stop and start the gateway without
terminating the requester's HTTP process.

## Supported service managers

| Platform | `--restart-manager` | Command and default target |
|---|---|---|
| Kubernetes (Linux) | `kubernetes` | API patch of `deployment/sparky` pod-template annotation |
| Linux (systemd) | `systemd` | `systemctl restart sparky.service` |
| Linux (OpenRC) | `openrc` | `rc-service sparky restart` |
| Linux (SysVinit) | `sysv` | `service sparky restart` |
| Linux (runit) | `runit` | `sv restart sparky` |
| Linux (s6) | `s6` | `s6-svc -r /run/service/sparky` |
| macOS (launchd) | `launchd` | `launchctl kickstart -k system/org.sparky.gateway` |
| Linux or macOS (other supervisor) | `custom` | Run one administrator-configured, absolute-path helper with zero arguments |

The default `--restart-manager auto` selects **Kubernetes** inside a Linux Pod
(when `KUBERNETES_SERVICE_HOST` is set), macOS launchd, or Linux systemd
(when `/run/systemd/system` exists). It **does not** attempt to guess which of
the other Linux init systems is actually supervising Sparky. For those, select
the manager explicitly. Missing binaries or unsupported platforms fail closed
during application startup.

Commands are always invoked directly without a shell. The browser never
provides the executable, arguments, service identifier, or helper path.
The server's `--restart-target` option can override the fixed default at
startup to accommodate the operator's installed service name. Examples:

```sh
# Linux using systemd:
sparky-config --enable-restart --restart-manager systemd

# Linux using OpenRC:
sparky-config --enable-restart --restart-manager openrc

# Linux using runit:
sparky-config --enable-restart --restart-manager runit

# Linux using s6 with service directory /run/service/sparky:
sparky-config --enable-restart --restart-manager s6

# macOS LaunchDaemon registered as system/org.sparky.gateway:
sparky-config --enable-restart --restart-manager launchd

# macOS LaunchAgent for UID 501, with the service already registered:
sparky-config --enable-restart --restart-manager launchd \
  --restart-target gui/501/org.sparky.gateway
```

The `system/` launchd domain typically requires privileged authorization.
A `gui/<uid>/` domain is a different, user-scoped service. Register the
correct LaunchDaemon or LaunchAgent in advance and grant the UI process the
minimum necessary authority. **Do not run the browser-facing web UI as root.**

### Kubernetes Deployment rollout

Inside a Kubernetes Pod, `--enable-restart` selects the Kubernetes manager
automatically. The default target is `deployment/sparky`; override at
**process startup** with `--restart-target deployment/<deployment-name>`.
The editor uses its mounted service-account token, namespace, and Kubernetes
CA bundle to send a single authenticated, TLS-validated Kubernetes API
merge PATCH to the Deployment's pod-template annotation. Kubernetes then
rolls out replacement Pods according to the Deployment strategy.

```sh
sparky-config --enable-restart --restart-manager kubernetes \
  --restart-target deployment/sparky
```

The UI must run in a Pod with a service-account token available, have network
access to the cluster API server, and be granted **only patch permission on
the intended Deployment** in its namespace. Example RBAC (replace namespace,
deployment and service-account name to match the installation):

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: sparky-restart
  namespace: radio
rules:
  - apiGroups: ["apps"]
    resources: ["deployments"]
    resourceNames: ["sparky"]
    verbs: ["patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: sparky-restart
  namespace: radio
subjects:
  - kind: ServiceAccount
    name: sparky-config
    namespace: radio
roleRef:
  kind: Role
  name: sparky-restart
  apiGroup: rbac.authorization.k8s.io
```

**Important:** The editor modifies the YAML file separately. If the YAML is
only stored in the container's writable layer or a ConfigMap mount, a rollout
may discard those edits (ConfigMap mounts are normally read-only). Use an
appropriate persistent, writable configuration volume or another durable
configuration deployment mechanism. A restart will not fix volatile or
read-only config storage.

This restart approach does **not** require access to other Pods' PIDs, host
PID namespaces, `kubectl`, permission to delete Pods, or a shell. It is
also safer than terminating a guessed PID. The API acknowledging the patch
does not mean the rollout completed or the new Pods became healthy.

For a bare process with no supervisor, there is nothing to restart it once it
exits. In that case, use a real service supervisor or an operator-provided
fixed helper that owns a reliable stop/start lifecycle; do **not** silently
spawn another Sparky process from the web handler.

### Other Linux supervisors / custom adapters

Any other init system can be integrated through an operator-installed fixed
helper, such as `/usr/local/libexec/sparky-restart`. The helper itself is a
separate deployment responsibility; it must restart **only Sparky** and should
be owned by a trusted administrator, not writable by the UI user.

```sh
sparky-config --enable-restart --restart-manager custom \
  --restart-helper /usr/local/libexec/sparky-restart
```

A custom helper gets **no HTTP-supplied arguments**, no shell invocation, and
no arguments from the YAML configuration. This makes the web endpoint
agnostic about unusual init systems without turning it into a remote
command executor. The helper must provide its own tight authorization and
avoid following untrusted file paths or executing unchecked content.

## Security boundary

- Restart is disabled unless `--enable-restart` is set.
- The authenticated route is `POST /restart`; GET is not accepted.
- Each POST requires the correct Origin and CSRF token plus an explicit
  confirmation checkbox.
- A maximum of one restart operation runs at a time per UI process.
- The operation has a 15-second timeout. An accepted request **does not prove
  the gateway came back healthy**, nor is it proof the command succeeded
  after the service manager detached.
- No sudo/polkit rule, password database, service unit or elevated identity
  is created by Sparky. Grant only the targeted restart authority outside
  the application. Do not grant unrestricted sudo or arbitrary service
  management to the UI user.
- The same HTTPS/loopback/Routy trusted-proxy restrictions apply. In
  particular, the Routy-to-Sparky LAN HTTP hop carries Basic credentials in
  plaintext unless further protected.
- Repeated *sequential* authenticated requests can restart the gateway
  again. This is an admin-only control, not rate-limited or a health monitor.

Restart never saves pending form values. First press **Save Changes**, then
use the separate **Restart Sparky** action, checking the confirmation box.
If system-manager permissions or the gateway service are misconfigured,
the UI shows a generic failure; examine the host's service-manager logs.

## Limitations and next steps

This stage does not integrate the web UI into the main gateway binary. Kubernetes is
supported specifically for Deployment-managed workloads; standalone Pods,
StatefulSets and DaemonSets require a separately designed/reviewed adapter.
It has no automatic health checks or rollback. The existing
`/edit?saved=1` banner confirms the save flow, not a reliable
running-vs-saved configuration comparison. The optional live status panel
shows health and delivery counts, **not** the active config revision. A true
running-versus-saved comparison would require separately designed runtime
configuration reporting.
