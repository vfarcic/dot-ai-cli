# User Management

Manage static Dex users through the CLI. These commands interact with the server's `/users` API.

## List Users

List all registered user emails:

```bash
dot-ai users list
```

## Create a User

Create a new static Dex user with an email and password:

```bash
dot-ai users create --email user@example.com --password secretpass
```

Both `--email` and `--password` are required flags.

## Delete a User

Delete a user by email:

```bash
dot-ai users delete user@example.com
```

The email is a required positional argument.

## When to Use Static Users

**Development and testing:** Static users are convenient for local development, CI pipelines, and testing environments where setting up an external identity provider is unnecessary.

**Production with SSO:** For production deployments, prefer configuring Dex with an external identity provider connector (LDAP, OIDC, SAML, GitHub, etc.). See the [identity provider connectors guide](https://devopstoolkit.ai/docs/ai-engine/setup/connectors) for setup instructions.

Static users and IdP connectors can coexist — use static users as break-glass accounts alongside your SSO connector.

## Next Steps

- **[Authentication](../setup/authentication.md)** — OAuth login flow and token management
- **[Configuration](../setup/configuration.md)** — Server URL and credential settings
- **[Commands Overview](cli-commands-overview.md)** — See all available commands
