import { Route, Routes } from "react-router-dom";
import { AppLayout } from "./components/AppLayout";
import { Dashboard } from "./routes/Dashboard";
import { HealthCheck } from "./routes/HealthCheck";
import { History } from "./routes/History";
import { Placeholder } from "./routes/Placeholder";
import { DashboardSummaryProvider } from "./context/DashboardSummaryContext";

export default function App() {
  return (
    <DashboardSummaryProvider>
      <Routes>
        <Route element={<AppLayout />}>
          <Route index element={<Dashboard />} />
          <Route path="fila" element={<Placeholder screen="Fila" phase="Fase B" />} />
          <Route
            path="aprovacao"
            element={<Placeholder screen="Aguardando Aprovação" phase="Fase B" />}
          />
          <Route
            path="diretorios"
            element={<Placeholder screen="Diretórios Monitorados" phase="Fase C" />}
          />
          <Route path="regras" element={<Placeholder screen="Regras" phase="Fase D" />} />
          <Route path="health-check" element={<HealthCheck />} />
          <Route path="historico" element={<History />} />
          <Route path="*" element={<Placeholder screen="Página não encontrada" phase="—" />} />
        </Route>
      </Routes>
    </DashboardSummaryProvider>
  );
}
