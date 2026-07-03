async function loadStatus() {
  const health = document.querySelector("#health-status");
  const dependencies = document.querySelector("#dependency-list");

  try {
    const healthResponse = await fetch("/api/health");
    const healthBody = await healthResponse.json();
    health.textContent = `${healthBody.service} is ${healthBody.status}`;
  } catch (error) {
    health.textContent = "Backend health is unavailable";
  }

  try {
    const readyResponse = await fetch("/api/ready");
    const readyBody = await readyResponse.json();
    dependencies.replaceChildren(
      ...Object.entries(readyBody.dependencies).map(([name, status]) => {
        const item = document.createElement("li");
        const detail = status.mode || status.url || "";
        item.textContent = `${name}: ${status.status}${detail ? ` (${detail})` : ""}`;
        return item;
      }),
    );
  } catch (error) {
    const item = document.createElement("li");
    item.textContent = "Dependency readiness is unavailable";
    dependencies.replaceChildren(item);
  }
}

loadStatus();
