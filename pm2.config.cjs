const path = require("path");

const root = __dirname;
const python = path.join(root, ".pixi", "envs", "default", "bin", "python");

module.exports = {
  apps: [
    {
      name: "qunce-server",
      cwd: root,
      script: python,
      args: "-m server.app.main",
      interpreter: "none",
      env: {
        QUNCE_SERVER_HOST: "0.0.0.0",
        QUNCE_SERVER_PORT: "8000",
      },
    },
  ],
};
