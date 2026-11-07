import { z } from 'zod';

export const PasskeyDomainSchema = z.object({
  domain: z.string().trim().min(1, 'passkey.invalidDomain').max(253, 'passkey.invalidDomain'),
});
export const PasskeyNameSchema = z.object({
  name: z.string().trim().min(1, 'passkey.invalidName').max(64, 'passkey.invalidName'),
});
export const PasskeyViewSchema = z.object({
  domain: z.string(),
  credentials: z.array(
    z.object({
      id: z.string(),
      name: z.string(),
      createdAt: z.number(),
      lastUsedAt: z.number(),
    }),
  ),
});
export type PasskeyDomain = z.infer<typeof PasskeyDomainSchema>;
export type PasskeyName = z.infer<typeof PasskeyNameSchema>;
export type PasskeyView = z.infer<typeof PasskeyViewSchema>;
