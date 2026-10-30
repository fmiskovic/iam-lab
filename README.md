# iam-lab

Runnable labs and experiments for the posts on **[iamfilip.dev](https://iamfilip.dev)**, a blog about Identity & Access Management in practice: Keycloak, OpenID Connect, SAML, Entra ID and running Keycloak on Kubernetes.

Every lab runs locally with Docker. Keycloak configuration is provisioned with Terraform, so you can read exactly what was set up and why.

| Lab | Post | Keycloak |
|---|---|---|
| [keycloak-token-exchange](keycloak-token-exchange/) | [Keycloak Token Exchange, Hands-On](https://iamfilip.dev/posts/keycloak-token-exchange/) | 26.8.1 |

```bash
git clone https://github.com/fmiskovic/iam-lab.git
cd iam-lab/<lab>
```

Each lab's README has the steps to run it.

Found a problem or have a question? [Open an issue](https://github.com/fmiskovic/iam-lab/issues).

## License

[MIT](LICENSE)
