export type WellKnown = {
  product: "withsecrets";
  apiVersion: "v1";
  instanceId: string;
  publicBaseUrl: string;
  authProviders: string[];
  cryptoVersion: 1;
};

export type ItemRecord = {
  id: string;
  vaultId: string;
  ciphertext: string;
  cryptoVersion: 1;
  revision: number;
  deleted: boolean;
};
