import adapter from "@sveltejs/adapter-static";
import { vitePreprocess } from "@sveltejs/vite-plugin-svelte";

/** @type {import('@sveltejs/kit').Config} */
const config = {
  preprocess: vitePreprocess(),
  kit: {
    paths: { relative: false },
    adapter: adapter({
      pages: "../server/portalui",
      assets: "../server/portalui",
      fallback: "index.html",
    }),
  },
};

export default config;
