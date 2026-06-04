document.documentElement.classList.add("js");

window.addEventListener("load", () => {
  const { gsap } = window;

  if (!gsap || window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
    document.documentElement.classList.remove("js");
    return;
  }

  const scrollTrigger = window.ScrollTrigger;
  if (scrollTrigger) {
    gsap.registerPlugin(scrollTrigger);
  } else {
    gsap.set(".capability-intro, .capability-grid article, .steps article, .feature-grid article, .video-grid article, .command-panel", { opacity: 1 });
  }

  const hero = document.querySelector(".hero");
  if (hero) {
    hero.addEventListener("pointermove", (event) => {
      const rect = hero.getBoundingClientRect();
      const x = ((event.clientX - rect.left) / rect.width) * 100;
      const y = ((event.clientY - rect.top) / rect.height) * 100;

      gsap.to(hero, {
        "--cursor-x": `${x}%`,
        "--cursor-y": `${y}%`,
        "--dither-x": `${(x - 50) * 0.18}px`,
        "--dither-y": `${(y - 50) * 0.18}px`,
        duration: 0.38,
        ease: "power2.out",
      });
    });

    hero.addEventListener("pointerleave", () => {
      gsap.to(hero, {
        "--cursor-x": "72%",
        "--cursor-y": "26%",
        "--dither-x": "0px",
        "--dither-y": "0px",
        duration: 0.7,
        ease: "power3.out",
      });
    });
  }

  const heroTimeline = gsap.timeline({ defaults: { ease: "power3.out" } });
  heroTimeline
    .from(".hero-github-card", { y: -80, opacity: 0, rotate: 4, duration: 0.78 })
    .from(".terminal-window-main", { x: 90, y: 30, rotateY: -12, opacity: 0, duration: 1.1 })
    .from(".terminal-window pre", { clipPath: "inset(0 100% 0 0)", duration: 1.15 }, "-=0.55")
    .from(".hero-content > *", { y: 34, opacity: 0, stagger: 0.11, duration: 0.72 }, "-=0.65")
    .from(".terminal-grid > div", { y: 24, opacity: 0, stagger: 0.12, duration: 0.62 }, "-=0.5");

  if (scrollTrigger) {
    gsap.to(".terminal-window-main", {
      yPercent: -8,
      rotateY: -2,
      ease: "none",
      scrollTrigger: {
        trigger: ".hero",
        start: "top top",
        end: "bottom top",
        scrub: true,
      },
    });
  }

  gsap.utils.toArray(".capability-intro, .capability-grid article, .section-heading, .steps article, .feature-grid article, .command-panel, .video-grid article").forEach((item) => {
    if (!scrollTrigger) {
      gsap.to(item, { y: 0, opacity: 1, duration: 0.4 });
      return;
    }

    gsap.fromTo(
      item,
      { y: 44, opacity: 0 },
      {
        y: 0,
        opacity: 1,
        duration: 0.75,
        ease: "power3.out",
        scrollTrigger: {
          trigger: item,
          start: "top 84%",
        },
      },
    );
  });

  gsap.utils.toArray(".button").forEach((button) => {
    button.addEventListener("pointermove", (event) => {
      const rect = button.getBoundingClientRect();
      const x = event.clientX - rect.left - rect.width / 2;
      const y = event.clientY - rect.top - rect.height / 2;
      gsap.to(button, { x: x * 0.18, y: y * 0.26, duration: 0.24, ease: "power2.out" });
    });

    button.addEventListener("pointerleave", () => {
      gsap.to(button, { x: 0, y: 0, duration: 0.45, ease: "elastic.out(1, 0.35)" });
    });
  });

  gsap.utils.toArray(".video-grid article").forEach((card) => {
    card.addEventListener("pointermove", (event) => {
      const rect = card.getBoundingClientRect();
      const px = (event.clientX - rect.left) / rect.width - 0.5;
      const py = (event.clientY - rect.top) / rect.height - 0.5;
      card.style.setProperty("--mx", `${px * 80}px`);
      card.style.setProperty("--my", `${py * 80}px`);
      gsap.to(card, {
        rotateY: px * 8,
        rotateX: py * -8,
        y: -8,
        duration: 0.28,
        ease: "power2.out",
      });
    });

    card.addEventListener("pointerleave", () => {
      gsap.to(card, { rotateY: 0, rotateX: 0, y: 0, duration: 0.55, ease: "elastic.out(1, 0.45)" });
    });
  });
});
