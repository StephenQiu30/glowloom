import { defineConfig } from 'prisma/config';

// Generation only. Structure is managed offline from backend/db/schema.sql.
export default defineConfig({ schema: 'prisma/schema.prisma' });
