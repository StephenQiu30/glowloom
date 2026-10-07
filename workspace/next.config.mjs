import nextra from "nextra";
import remarkSourceLinks from "./lib/remark-source-links.mjs";

const withNextra = nextra({
  mdxOptions: { remarkPlugins: [remarkSourceLinks] },
});

export default withNextra({
  agentRules: false,
  devIndicators: false,
  allowedDevOrigins: ["127.0.0.1"],
  reactStrictMode: true,
  experimental: { cpus: 2 },
  webpack(config, { webpack }) {
    // Nextra's dynamic content import must not compile attachments or local state.
    config.plugins.push(
      new webpack.ContextReplacementPlugin(
        /[/\\]workspace[/\\]content$/,
        /^\.\/(?![._])(?!.*\/[._]).*\.mdx?$/,
      ),
    );
    return config;
  },
});
