import tailwindcss from "@tailwindcss/vite";
import { sveltekit } from "@sveltejs/kit/vite";
import { defineConfig } from "vite-plus";

export default defineConfig({
  plugins: [sveltekit(), tailwindcss()],
});
