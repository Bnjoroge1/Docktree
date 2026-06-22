const express = require('express');
const app = express();
const PORT = 80;

app.get('/', (req, res) => {
  res.json({
    status: 'ok',
    message: 'Docktree test UI',
    zone: process.env.ZONE || 'default',
    apiUrlA: process.env.API_URL_A || 'not set',
    apiUrlB: process.env.API_URL_B || 'not set',
  });
});

app.get('/health', (req, res) => {
  res.json({ healthy: true });
});

app.listen(PORT, () => {
  console.log(`UI listening on port ${PORT}`);
});
