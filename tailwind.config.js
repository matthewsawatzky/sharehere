/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./internal/webui/templates/*.html",
    "./internal/webui/static/*.js",
    "./internal/server/*.go"
  ],
  theme: {
    extend: {
      boxShadow: {
        panel: "0 10px 24px rgba(2, 6, 23, 0.08)"
      }
    }
  },
  plugins: []
};
