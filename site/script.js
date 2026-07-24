(function () {
  const header = document.querySelector("[data-site-header]");
  const copyButtons = document.querySelectorAll("[data-copy-target]");
  const tabButtons = document.querySelectorAll("[data-example-tab]");
  const panels = document.querySelectorAll("[data-example-panel]");
  const filterDemo = document.querySelector("[data-filter-demo]");

  function syncHeader() {
    if (!header) {
      return;
    }

    header.classList.toggle("is-scrolled", globalThis.scrollY > 12);
  }

  async function copyText(button) {
    const targetId = button.dataset.copyTarget;
    const target = document.getElementById(targetId);
    const card = button.closest("[data-copy-card]");
    const status = card?.querySelector("[data-copy-status]");

    if (!target) {
      return;
    }

    const text = target.textContent.trim();

    try {
      if (!navigator.clipboard?.writeText) {
        throw new Error("Clipboard API is unavailable");
      }
      await navigator.clipboard.writeText(text);
      button.classList.add("is-copied");
      button.setAttribute("aria-label", "Install command copied");
      if (status) {
        status.textContent = "Install command copied.";
      }
      globalThis.setTimeout(function () {
        button.classList.remove("is-copied");
        button.setAttribute("aria-label", "Copy install command");
        if (status) {
          status.textContent = "";
        }
      }, 4200);
    } catch (error) {
      console.error("Copy failed", error);
      if (status) {
        status.textContent = "Copy failed. Select the command manually.";
      }
    }
  }

  function activateExample(name) {
    tabButtons.forEach(function (button) {
      const selected = button.dataset.exampleTab === name;
      button.classList.toggle("is-active", selected);
      button.setAttribute("aria-selected", selected ? "true" : "false");
      button.setAttribute("tabindex", selected ? "0" : "-1");
    });

    panels.forEach(function (panel) {
      panel.hidden = panel.dataset.examplePanel !== name;
    });
  }

  function setupFilterDemo() {
    if (!filterDemo) {
      return;
    }

    filterDemo.querySelectorAll(".filter-demo-source code, .filter-demo-result code").forEach(function (code) {
      Array.prototype.slice.call(code.childNodes).forEach(function (node) {
        if (node.nodeType === 3) {
          node.remove();
        }
      });
    });
    filterDemo.classList.add("is-enhanced");

    if (globalThis.matchMedia("(prefers-reduced-motion: reduce)").matches) {
      if (filterDemo) {
        filterDemo.querySelectorAll(".filter-demo-source [data-filter-show]").forEach(function (line) {
          line.classList.add("is-active");
        });
        filterDemo.querySelectorAll(".filter-demo-result [data-filter-hide]").forEach(function (line) {
          line.classList.add("is-hidden");
        });
      }
      return;
    }

    const stage = filterDemo.querySelector("[data-filter-stage]");
    const result = filterDemo.querySelector("[data-filter-result]");
    const filterLines = filterDemo.querySelectorAll(".filter-demo-source [data-filter-show]");
    const outputLines = filterDemo.querySelectorAll(".filter-demo-result [data-filter-hide]");
    const steps = [
      { stage: "start with the command", result: "native output" },
      { stage: "match the invocation", result: "native output" },
      { stage: "drop the banner", result: "less noise" },
      { stage: "drop runner noise", result: "even less noise" },
      { stage: "drop timing noise", result: "compact output" },
    ];
    let stepIndex = 0;
    let timer = null;

    function setFilterLineState(line, visible, animate) {
      if (!animate) {
        line.classList.toggle("is-active", visible);
        return;
      }

      globalThis.requestAnimationFrame(function () {
        line.classList.toggle("is-active", visible);
      });
    }

    function renderFilterStep(step, animate) {
      filterLines.forEach(function (line) {
        setFilterLineState(line, Number(line.dataset.filterShow) <= step, animate !== false);
      });
      outputLines.forEach(function (line) {
        line.classList.toggle("is-hidden", Number(line.dataset.filterHide) <= step);
      });
      if (stage) {
        stage.textContent = steps[step].stage;
      }
      if (result) {
        result.textContent = steps[step].result;
      }
    }

    function start() {
      if (timer) {
        return;
      }
      renderFilterStep(stepIndex, false);
      timer = globalThis.setInterval(function () {
        stepIndex = (stepIndex + 1) % steps.length;
        renderFilterStep(stepIndex);
      }, 1800);
    }

    function stop() {
      if (!timer) {
        return;
      }
      globalThis.clearInterval(timer);
      timer = null;
    }

    if (!("IntersectionObserver" in globalThis)) {
      renderFilterStep(steps.length - 1, false);
      return;
    }

    const observer = new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (entry.isIntersecting) {
          start();
        } else {
          stop();
        }
      });
    }, { threshold: 0.45 });

    observer.observe(filterDemo);
    renderFilterStep(stepIndex, false);
  }

  syncHeader();
  globalThis.addEventListener("scroll", syncHeader, { passive: true });

  copyButtons.forEach(function (button) {
    button.addEventListener("click", function () {
      copyText(button);
    });
  });

  tabButtons.forEach(function (button) {
    button.addEventListener("click", function () {
      activateExample(button.dataset.exampleTab);
    });

    button.addEventListener("keydown", function (event) {
      const index = Array.prototype.indexOf.call(tabButtons, button);
      let nextIndex;

      if (event.key === "ArrowRight") {
        nextIndex = (index + 1) % tabButtons.length;
      } else if (event.key === "ArrowLeft") {
        nextIndex = (index - 1 + tabButtons.length) % tabButtons.length;
      } else if (event.key === "Home") {
        nextIndex = 0;
      } else if (event.key === "End") {
        nextIndex = tabButtons.length - 1;
      } else {
        return;
      }
      
      event.preventDefault();
      const nextButton = tabButtons[nextIndex];
      activateExample(nextButton.dataset.exampleTab);
      nextButton.focus();
    });
  });

  setupFilterDemo();
})();
