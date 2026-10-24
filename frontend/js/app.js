(() => {
  const guest = document.getElementById("guest");
  const userPanel = document.getElementById("user");
  const errorEl = document.getElementById("error");
  const dialog = document.getElementById("consent-dialog");
  const params = new URLSearchParams(location.search);

  if (params.get("auth_error")) {
    showError("Ошибка входа: " + params.get("auth_error"));
  }

  document.getElementById("logout").addEventListener("click", async () => {
    await fetch("/auth/logout", { method: "POST", credentials: "same-origin" });
    location.href = "/";
  });

  document.getElementById("consent-decline").addEventListener("click", async () => {
    dialog.close();
    await fetch("/auth/logout", { method: "POST", credentials: "same-origin" });
    location.href = "/";
  });

  document.getElementById("consent-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    const check = document.getElementById("consent-check");
    if (!check.checked) return;
    const res = await fetch("/auth/consent", {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ accepted: true }),
    });
    if (!res.ok) {
      showError("Не удалось сохранить согласие");
      return;
    }
    dialog.close();
    history.replaceState({}, "", "/");
    await refresh();
  });

  async function refresh() {
    const res = await fetch("/auth/me", { credentials: "same-origin" });
    const data = await res.json();
    const u = data.user;

    if (!u) {
      guest.classList.remove("hidden");
      userPanel.classList.add("hidden");
      return;
    }

    guest.classList.add("hidden");
    userPanel.classList.remove("hidden");

    document.getElementById("name").textContent = u.display_name || "Пользователь";
    document.getElementById("email").textContent = u.email || u.oauth_provider;
    const avatar = document.getElementById("avatar");
    if (u.avatar_url) {
      avatar.src = u.avatar_url;
      avatar.classList.remove("hidden");
    } else {
      avatar.removeAttribute("src");
      avatar.classList.add("hidden");
    }

    if (data.privacy_policy_version) {
      document.getElementById("policy-ver").textContent = data.privacy_policy_version;
    }

    if (data.needs_consent || params.get("consent") === "1") {
      dialog.showModal();
    }
  }

  function showError(msg) {
    errorEl.textContent = msg;
    errorEl.classList.remove("hidden");
  }

  refresh().catch(() => showError("Не удалось загрузить сессию"));
})();
