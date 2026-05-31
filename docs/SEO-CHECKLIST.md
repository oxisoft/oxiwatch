# OxiWatch SEO / discoverability checklist

Things the code can't set for you. Do these in the GitHub web UI and around the web.
Order matters: 1 to 3 are the highest-leverage and take ~10 minutes total.

## 1. Repo "About" box (Settings is the gear icon next to "About" on the repo home)

**Description** (paste this, keyword-front-loaded, 138 chars):

```
SSH login monitor for Linux: instant Telegram alerts on every login + daily report of failed brute-force attempts, with GeoIP.
```

**Website:** `https://oxisoft.io`  *(or `https://oxiwatch.oxisoft.io` once Pages is live)*

**Topics** (add all, each is its own discoverable GitHub topic page):

```
ssh  ssh-monitoring  telegram-bot  telegram-notifications  linux-security
sshd  intrusion-detection  fail2ban  geoip  golang  systemd
server-security  sysadmin  devops  security-tools
```

## 2. Enable GitHub Pages (the landing site)

- **Settings → Pages → Build and deployment → Source: Deploy from a branch**
- Branch: `main`, folder: `/docs` → Save.
- The repo includes `docs/CNAME` set to `oxiwatch.oxisoft.io`. For that to resolve, add a DNS record at your domain host:
  - `CNAME  oxiwatch  →  oxisoft.github.io`
- **Don't want a custom domain?** Delete `docs/CNAME`, and the site serves at
  `https://oxisoft.github.io/oxiwatch/` instead. (If you do this, update the
  `canonical`, `og:url`, and sitemap URLs in `docs/index.html` + `docs/sitemap.xml`
  to the github.io address.)
- Tick **Enforce HTTPS** once the cert is issued.

## 3. Social preview image

- Create `docs/assets/og.png` at **1280×640** (see `docs/assets/README.md`).
- Upload it under **Settings → General → Social preview**. This is what shows when
  the repo is shared on X, Slack, Telegram, LinkedIn. Big CTR lever.

## 4. Make a fresh release with good notes

Search engines and GitHub rank active repos higher. When you cut the next release,
write a real changelog entry (you already do in `CHANGELOG.md`). A recent release
date on the repo home helps both Google freshness and GitHub's own ranking.

## 5. Backlinks (the actual driver of Google ranking for the brand term)

A new repo has little authority; backlinks from trusted sites are what move
"oxiwatch" to #1 and surface it for functional searches.

- [ ] **oxisoft.io project page**: add a page for OxiWatch on your company site that
      links to the repo and the landing page. Your own domain's authority passes through.
- [ ] **awesome-selfhosted**: PR under "Security" / "Monitoring".
- [ ] **awesome-go**: PR under "Security" or "DevOps".
- [ ] **awesome-sysadmin / awesome-security**: PRs where it fits.
- [ ] **alternativeto.net**: list it (tie it to fail2ban / SSH-monitoring searches).
- [ ] **LibHunt / Awesome Go (libhunt)**: submit the repo.
- [ ] **Show HN / r/selfhosted / r/linuxadmin / r/homelab**: a short "I built X" post.
      These drive real traffic + natural links. Ask and I'll draft the copy.

## 6. Consistency (helps brand disambiguation)

- Always write the product as **OxiWatch** (one word, capital O and W).
- Use the same one-line description everywhere (repo, Pages, oxisoft.io, directories).
- Same OG image everywhere so the brand card is recognizable.

---

### Quick reference: target keywords already woven into README + landing page
`ssh login monitor` · `ssh login telegram notification` · `monitor ssh logins linux`
`sshd login alert` · `failed ssh login report` · `ssh brute force notification`
`telegram ssh alert` · `geoip ssh login` · `ssh security monitoring debian ubuntu`
