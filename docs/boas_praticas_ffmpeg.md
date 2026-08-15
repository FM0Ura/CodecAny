# **Engenharia de Transcodificação e Preservação Digital: Boas Práticas, Parâmetros e Exceções no Uso do FFmpeg para H.265 e AV1**

## **Arquitetura do Processamento de Mídia: Remuxing vs. Transcodificação**

A manipulação de arquivos multimídia através da ferramenta FFmpeg exige uma diferenciação conceitual rigorosa entre a reencapsulação de contêiner (*remuxing*) e a retranscodificação de fluxos elementares1. O fluxo de origem codificado em H.264 (Advanced Video Coding \- AVC), proveniente de capturas de alta taxa de bits ou extrações diretas de mídias ópticas (*H.264 Remux*), contém estruturas de sintaxe e vetores de movimento que definem a representação espacial e temporal do vídeo.  
O processo de *remuxing* altera estritamente a camada de multiplexação (como a transição do encapsulamento MP4 para Matroska MKV) sem modificar a carga útil dos fluxos de vídeo, áudio ou legendas2. Essa operação possui baixíssima demanda computacional e resulta em preservação matemática absoluta do sinal original (-c copy)2. Em contrapartida, a transcodificação para H.265 (High Efficiency Video Coding \- HEVC) ou AV1 (AOMedia Video 1\) exige o decodamento completo do fluxo H.264 para o domínio YUV bruto, seguido pela aplicação de novos algoritmos de estimação de movimento, transformada discreta de cosseno (DCT) ou transformada direcional e quantização5.  
O pipeline de processamento do FFmpeg opera em estágios bem delimitados. Inicialmente, os demuxers realizam a leitura do contêiner de entrada (como MKV, MP4 ou TS) e separam os fluxos elementares de vídeo, áudio, legendas e metadados1. No caminho da transcodificação, o decodificador decomprime o vídeo H.264 em quadros brutos no espaço de cor YUV8. Esses quadros passam pelos encoders selecionados (libx265 ou libsvtav1), onde sofrem nova compressão5. Paralelamente, no caminho de cópia direta (*stream copy*), os fluxos de áudio e legendas podem ignorar os decodificadores e reencodificadores, fluindo diretamente para os muxers que montam o contêiner final2.  
Por padrão, a heurística de seleção de fluxos do FFmpeg adota um comportamento restritivo: seleciona apenas um fluxo de cada tipo (o vídeo de maior resolução, o áudio com maior número de canais e a primeira legenda)1. Em cenários profissionais e de preservação, a perda não intencional de faixas secundárias de áudio (como dublagens ou comentários) e de legendas é inaceitável. O controle sobre a topologia do arquivo de saída exige a aplicação explícita do parâmetro de mapeamento \-map 0, garantindo a transferência integral de todos os fluxos presentes no contêiner primário3.  
Quando o objetivo é converter o fluxo de vídeo para H.265 ou AV1 mantendo a integridade absoluta das faixas de áudio surround (como DTS-HD Master Audio, Dolby TrueHD ou LPCM) e legendas, o pipeline deve isolar a reencodificação do vídeo enquanto instrui o pass-through dos demais componentes11. A adição do sufixo opcional ? aos especificadores de fluxo (como \-map 0:s? ou \-map 0:t?) impede que a execução seja interrompida caso o arquivo de origem não possua anexos específicos, como fontes tipográficas integradas ao contêiner Matroska3.

## **Regimes de Conversão: Transcodificação com Perda (Lossy)**

A transcodificação com perda fundamenta-se na eliminação de redundâncias espaciais, temporais e psicovisuais que não são perceptíveis ao sistema visual humano em condições normais de visualização6. A transição de H.264 para H.265 ou AV1 permite reduções substanciais de armazenamento mantendo a fidelidade perceptual6.

### **Codificação H.265 (HEVC) via libx265**

A biblioteca libx265 gerencia a alocação de bits através de múltiplos modos de controle de taxa10. O modo *Constant Rate Factor* (CRF) é a abordagem recomendada para fluxos de trabalho que priorizam a qualidade constante sem restrição rígida do tamanho final do arquivo8. Diferente do modo de taxa de bits constante (CBR), o CRF varia a quantização dinamicamente quadro a quadro, aumentando a compressão em cenas de alto movimento e preservando detalhes em cenas estáticas complexas8.  
A escala de CRF do libx265 varia de 0 a 5110. Devido ao ganho de eficiência do HEVC em relação ao AVC, o valor de CRF em H.265 não possui equivalência numérica direta com H.26410. O parâmetro CRF 28 no libx265 oferece uma qualidade visual equivalente ao CRF 23 no libx264, produzindo tipicamente metade do tamanho de arquivo10. Para preservação visualmente transparente de fontes H.264 de alta qualidade, a faixa operacional ideal situa-se entre CRF 18 e CRF 22\.  
Os presets (ultrafast a veryslow) regulam o equilíbrio entre a velocidade de processamento e a eficiência da compressão10. Presets mais lentos ativam algoritmos avançados de estimação de movimento, partição de blocos transformados e análise psicovisual, resultando em menor tamanho de arquivo para o mesmo valor de CRF10. O preset placebo deve ser ignorado em ambientes de produção devido aos retornos marginais irrisórios em relação ao aumento exponencial do tempo de CPU10.  
Ao transcodificar fontes H.264 de 8 bits (![][image1]), é uma boa prática forçar a saída do H.265 em 10 bits (![][image2]) através de \-pix\_fmt yuv420p10le8. A precisão interna de 10 bits no libx265 reduz a ocorrência de artefatos de gradiente de cor (*banding*) nas etapas de quantização, além de fornecer maior margem matemática para transformadas sem custos significativos de bitrate8.

### **Codificação AV1 via libsvtav1**

O codificador *Scalable Video Technology for AV1* (libsvtav1) representa o estado da arte na distribuição eficiente de vídeo baseada em padrões abertos5. O intervalo de CRF do SVT-AV1 estende-se habitualmente de 1 a 63 (expandido até 70 em atualizações recentes da API)5. Para resolução 1080p e origens H.264 de alta definição, o valor inicial de CRF recomendado situa-se em torno de 28 a 32, o qual produz retenção detalhada com taxas de dados extremamente reduzidas5.  
A hierarquia de presets no SVT-AV1 opera de 0 a 1319. Os presets mais baixos (0 a 3\) exigem alto poder computacional e são voltados para distribuições *Video on Demand* (VOD) profissionais, enquanto a faixa entre 4 e 6 é o ponto ideal para transcodificação de uso pessoal e arquivamento19.  
O parâmetro de ajuste psicovisual e métrico é definido através do argumento \-svtav1-params tune=05. A opção tune=0 ativa a otimização para qualidade visual perceptiva (VQ), aprimorando a nitidez de bordas e retenção de detalhes finos, enquanto tune=1 otimiza o pico de relação sinal-ruído (PSNR), o que frequentemente suaviza a imagem em detrimento da fidelidade perceptiva observável5.

### **Síntese de Granulação de Filme (Film Grain Synthesis \- FGS)**

A compressão tradicional de grão de filme e ruído térmico em fontes H.264 de alta taxa de bits é ineficiente5. O ruído espacial representa uma frequência alta aleatória que força a alocação maciça de coeficientes de transformada quantizados em cada quadro, resultando em artefatos de bloco (*blocking*) ou taxas de bits desproporcionais6.  
O AV1 resolve essa limitação introduzindo a Síntese de Granulação de Filme (*Film Grain Synthesis*)5. O pipeline FGS funciona removendo a granulação da imagem através de um filtro de atenuação e analisando suas características estatísticas6. O sinal limpo é codificado com alta eficiência e os parâmetros de ruído são gravados como metadados nos cabeçalhos do fluxo de bits6. Durante a reprodução, o decodificador recria sinteticamente a textura do grão sobre o vídeo reconstruído6.  
A ativação do FGS no SVT-AV1 é executada via instrução \-svtav1-params film-grain=X:film-grain-denoise=115. A escala do parâmetro film-grain varia de 0 a 5015:

* Valores entre 4 e 8 aplicam-se a conteúdos de animação ou capturas digitais limpas15.  
* Valores entre 8 e 15 aplicam-se a produções gravadas em película de 35mm com ruído moderado15.  
* Valores entre 20 e 50 reservam-se a digitalizações ruidosas de 16mm ou películas históricas15.

A flag film-grain-denoise=1 garante que o codificador filtre o ruído do sinal antes da compressão15. Sem este parâmetro, o codificador tenta comprimir o ruído real e aplica a granulação sintética sobreposta, elevando a taxa de bits e gerando aberrações visuais14.  
Existe uma exceção metodológica crucial em relação às métricas objetivas de qualidade (como VMAF, PSNR e SSIM) ao utilizar o FGS14. Como a granulação sintética é gerada de forma probabilística durante a exibição, o desalinhamento de pixels entre o vídeo original e o sintetizado resulta em pontuações VMAF artificialmente degradadas14. O uso de métricas estatísticas para avaliar fontes codificadas com FGS produz resultados inconsistentes, sendo o julgamento psicovisual a referência válida14.

## **Regimes de Conversão: Transcodificação sem Perda (Lossless)**

A codificação sem perda (*lossless*) garante a reconstrução matemática exata de cada pixel da fonte original no momento do decodamento. Existe uma distinção técnica fundamental entre um fluxo "visualmente sem perda" (*visually lossless*) e um fluxo matematicamente sem perda (*true lossless*)8.  
Uma conversão com valor de CRF baixo (por exemplo, H.264 em CRF 17 ou H.265 em CRF 15\) descarta frequências imperceptíveis e altera valores discretos de matrizes de quantização, não permitindo recuperar a cadeia de bits exata da fonte8. Já o modo *true lossless* desativa por completo os estágios de quantização e aproximação, efetuando o bypass de transformadas10.

### **H.265 Lossless**

No libx265, a codificação sem perda **não** é garantida apenas pela aplicação do parâmetro \-crf 010. O mecanismo de codificação sem perda no HEVC requer a ativação explícita da opção lossless=1 passada diretamente para a biblioteca do codificador via \-x265-params lossless=110.  
Ao ativar lossless=1, o codificador libx265 grava informações no console indicando a razão de compressão sem perda (*lossless compression ratio*)10. A escolha do perfil e subamostragem de croma limita a compatibilidade do modo sem perda8. Para garantir a preservação exata, o formato de pixel deve corresponder à profundidade de cor e plano de croma da fonte8. Se o arquivo de entrada estiver em ![][image1], deve-se manter \-pix\_fmt yuv420p8. A conversão não intencional para ![][image3] durante uma codificação sem perda introduz transformações no plano de croma que aumentam desnecessariamente o tamanho do arquivo sem adicionar informação espacial real.

### **AV1 Lossless**

A implementação do modo sem perda no ecossistema AV1 varia conforme a biblioteca utilizada. O codificador de referência libaom-av1 suporta a sinalização nativa do modo sem perda atribuindo \-crf 0 ou injetando \-aom-params lossless=114.  
No caso do libsvtav1, o suporte a codificação sem perda foi integrado progressivamente em versões recentes do ecossistema18. Nas compilações anteriores à versão 3.0, o foco arquitetural do SVT-AV1 concentrava-se na compressão perceptiva com perda14. Para garantir a preservação estrita sem perda em AV1 usando compilações genéricas do FFmpeg, o libaom-av1 continua sendo o motor de referência padronizado14.

## **Comparativo Estruturado de Parâmetros e Modos de Codificação**

A tabela abaixo sintetiza a matriz de configurações operacionais para transcodificação no FFmpeg, diferenciando estratégias para eficiência de compressão e preservação sem perda.

| Categoria do Codificador | Motor (-c:v) | Modo de Taxa de Bits | Intervalo de Parâmetro | Preset Recomendado | Configuração Sem Perda (Lossless) | Formato de Pixel Alvo | Parâmetros Específicos do Codificador |
| :---- | :---- | :---- | :---- | :---- | :---- | :---- | :---- |
| **H.265 (HEVC) Lossy** | libx265 | CRF | CRF 18 – 248 | slow / slower \[cite: 10\] | N/A | yuv420p10le \[cite: 9, 23\] | \-x265-params open-gop=0 \[cite: 9\] |
| **H.265 (HEVC) Lossless** | libx265 | Transform Bypass | N/A | medium / slow \[cite: 10\] | \-x265-params lossless=1 \[cite: 10\] | Copiar da Fonte (yuv420p / 10le)8 | \-x265-params "lossless=1:open-gop=0" \[cite: 9, 10\] |
| **AV1 Perceptual Lossy** | libsvtav1 | CRF | CRF 26 – 345 | preset 4 – 6 \[cite: 19, 21\] | N/A | yuv420p10le \[cite: 15, 19\] | \-svtav1-params tune=0 \[cite: 5, 19\] |
| **AV1 Film Grain Lossy** | libsvtav1 | CRF \+ FGS | CRF 28 – 3515 | preset 4 – 6 \[cite: 15, 19\] | N/A | yuv420p10le \[cite: 15, 19\] | \-svtav1-params tune=0:film-grain=12:film-grain-denoise=1 \[cite: 15\] |
| **AV1 Lossless (AOM)** | libaom-av1 | Constant Quality | \-crf 0 \[cite: 20\] | preset 4 / 3 | \-crf 0 ou \-aom-params lossless=1 \[cite: 20\] | Copiar da Fonte (yuv420p)8 | \-aom-params lossless=1 \[cite: 20\] |
| **H.265 Hardware** | hevc\_nvenc | ConstQP / CQP | CQP 18 – 2324 | p6 / p7 (Quality) | \-preset lossless \[cite: 24\] | yuv420p10le / p010le | Opções específicas do SDK NVENC24 |

## **Exceções Técnicas, Incompatibilidades de Contêiner e Mitigação de Bugs**

A transcodificação entre ecossistemas de codecs introduz casos de borda e comportamentos anômalos que demandam intervenção direta nos parâmetros da linha de comando.

### **O Bug do Open GOP em libx265 e Inestabilidade de Temporal Seeking**

Por padrão, a biblioteca libx265 utiliza uma estrutura de *Group of Pictures* aberta (*Open GOP*)9. Em um Open GOP, os quadros B situados no início de um GOP podem referenciar quadros P ou I pertencentes ao GOP imediatamente anterior9. Embora isso proporcione um ganho marginal em termos de taxa de bits em relação ao *Closed GOP*, a estrutura cria severos problemas de compatibilidade9.  
Quando arquivos H.265 gravados com Open GOP são encapsulados no contêiner MP4 ou consumidos em softwares de edição não-linear (como Magix Vegas, Adobe Premiere ou leitores baseados em VirtualDub), ocorrem erros de interpretação no decodificador9:

* Falhas de busca e navegação (*seek-by-frame* com saltos para quadros incorretos)9.  
* Perda pontual dos dois últimos quadros da sequência de vídeo9.  
* Congelamentos ou artefatos de renderização no momento da transição de cenas9.

Para contornar completamente essa instabilidade em ambientes de pós-produção ou arquivamento executados no FFmpeg, é obrigatório desativar o Open GOP passando a instrução explicitamente para o codificador via \-x265-params open-gop=09.

### **Estrutura do GOP e Inserção de Keyframes**

A alocação de quadros-chave (Keyframes / Intra-frames) determina a precisão de busca temporal e a recuperação contra erros de transmissão5. Se o parâmetro \-g (tamanho do GOP) não for declarado, o FFmpeg adota valores genéricos que podem resultar em intervalos de busca longos (ex.: 250 quadros)5.  
Para distribuição profissional e servidores multimídia locais, recomenda-se parametrizar o tamanho do GOP para que a inserção de quadros-chave ocorra a cada 1 a 2 segundos5. Em vídeos com taxa de quadros de 23.976 fps ou 24 fps, um valor de \-g 48 ou \-g 120 garante um equilíbrio entre eficiência e capacidade de resposta5. Para fluxos de edição em que cada quadro precisa ser indexável de forma isolada (*Intra-frame encoding*), utiliza-se \-g 110.

### **Aceleração por Hardware vs. Codificação em Software**

A utilização de codificadores baseados em hardware integrado em GPUs (NVIDIA NVENC, AMD AMF, Intel QSV) altera a relação entre qualidade visual, tamanho de arquivo e desempenho computacional24. Os blocos funcionais ASIC das GPUs executam o processo de codificação com throughput alto, liberando os núcleos de processamento principal (CPU)19.  
No entanto, a eficiência de compressão medida pela curva BD-Rate nos codificadores por hardware é inferior à dos motores em software como libx265 e libsvtav117. Para obter a mesma qualidade visual perceptiva de um encode em software codificado com libx265 em CRF 20, o codificador hevc\_nvenc exige uma taxa de bits tipicamente de 15% a 30% maior17. Adicionalmente, codificadores por hardware possuem limitações de personalização e desativam recursos avançados de modelagem psicovisual, tais como a Síntese de Granulação de Filme do AV16.  
Acelerações por hardware devem ser reservadas para aplicações de streaming em tempo real, captura de tela ou processamento em massa de dados onde o tempo de execução é mais crítico do que a eficiência de espaço em disco19. Para finalidades de preservação, distribuição final e arquivamento, os codificadores em software são a escolha adequada10.

## **Sintaxe Operacional Recomendada e Fluxos de Trabalho Automatizáveis**

Abaixo apresentam-se os modelos de execução em linha de comando padronizados para as diferentes abordagens de conversão abordadas.

### **Cenário 1: Remux Direto sem Reencodificação**

Preservação integral de todos os fluxos de vídeo, áudio, legendas e anexos trocando a camada de encapsulamento para Matroska sem alterar os dados3.

Bash  
ffmpeg \-y \-i "input\_h264.mp4" \\  
  \-map 0 \\  
  \-c copy \\  
  \-map\_metadata 0 \\  
  "output\_remux.mkv"

### **Cenário 2: Conversão H.264 para H.265 10-bit com Perda (Lossy)**

Transcodificação do vídeo mantendo todos os fluxos de áudio e legendas intocados, forçando a precisão de 10 bits no H.265 e prevenindo o erro de Open GOP9.

Bash  
ffmpeg \-y \-i "input\_h264.mkv" \\  
  \-map 0:v \-map 0:a \-map 0:s? \-map 0:t? \\  
  \-c:v libx265 \\  
  \-crf 20 \\  
  \-preset slow \\  
  \-pix\_fmt yuv420p10le \\  
  \-x265-params "open-gop=0" \\  
  \-c:a copy \\  
  \-c:s copy \\  
  "output\_h265\_lossy.mkv"

### **Cenário 3: Conversão H.264 para AV1 10-bit com Síntese de Granulação (FGS)**

Fluxo otimizado para fontes ruidosas ou digitalizações de película cinema 35mm utilizando o algoritmo de reconstrução sintética do SVT-AV115.

Bash  
ffmpeg \-y \-i "input\_h264\_grain.mkv" \\  
  \-map 0 \\  
  \-c:v libsvtav1 \\  
  \-crf 28 \\  
  \-preset 5 \\  
  \-pix\_fmt yuv420p10le \\  
  \-svtav1-params "tune=0:film-grain=10:film-grain-denoise=1" \\  
  \-g 120 \\  
  \-c:a copy \\  
  \-c:s copy \\  
  "output\_av1\_fgs.mkv"

### **Cenário 4: Conversão H.264 para H.265 Matematicamente Sem Perda (Lossless)**

Preservação estrita dos pixels sem perdas de quantização mantendo a integridade exata do formato de cor da fonte8.

Bash  
ffmpeg \-y \-i "input\_h264\_master.mkv" \\  
  \-map 0 \\  
  \-c:v libx265 \\  
  \-x265-params "lossless=1:open-gop=0" \\  
  \-preset slow \\  
  \-pix\_fmt yuv420p \\  
  \-c:a copy \\  
  \-c:s copy \\  
  "output\_h265\_lossless.mkv"

### **Cenário 5: Transcodificação H.265 em 2-Passes direcionada a Taxa de Bits Alvo (VOD/Target Size)**

Nos casos em que o arquivo final precisa ter um tamanho estritamente delimitado, a codificação CRF deve ser substituída pela arquitetura de 2-passes via libx26510. O primeiro passe analisa a complexidade do vídeo e grava estatísticas em arquivo de log, enquanto o segundo passe aloca os bits com eficiência10.

Bash  
ffmpeg \-y \-i "input\_h264.mkv" \\  
  \-map 0:v \\  
  \-c:v libx265 \\  
  \-b:v 3500k \\  
  \-x265-params "pass=1:open-gop=0" \\  
  \-an \-f null /dev/null && \\  
ffmpeg \-y \-i "input\_h264.mkv" \\  
  \-map 0 \\  
  \-c:v libx265 \\  
  \-b:v 3500k \\  
  \-x265-params "pass=2:open-gop=0" \\  
  \-c:a copy \\  
  \-c:s copy \\  
  "output\_h265\_2pass.mkv"

## **Conclusões e Recomendações Técnicas**

A seleção da estratégia de conversão no FFmpeg deve ser guiada pela finalidade do projeto, equilibrando requisitos de compatibilidade, consumo de armazenamento e fidelidade de imagem.  
A preservação em nível de arquivo mestre exige o modo sem perda direto (-x265-params lossless=1 ou libaom-av1 \-crf 0\) aliado ao mapeamento integral de fluxos (-map 0\)4. Essa abordagem garante que nenhuma informação espacial seja descartada e que faixas secundárias de áudio e legendas permaneçam intactas4. O uso do formato de pixel correspondente ao sinal de origem previne elevações desnecessárias no tamanho do arquivo final8.  
Para cenários de distribuição e armazenamento de alta densidade, a combinação do codificador libsvtav1 com a Síntese de Granulação de Filme (film-grain) estabelece o padrão mais eficiente para a conversão de fontes H.264 ruidosas ou provenientes de películas6. A utilização da instrução de ajuste visual \-svtav1-params tune=0 garante superioridade na retenção de detalhes finos e nitidez em relação aos modos focados em métricas matemáticas5.  
Por fim, a mitigação de falhas de compatibilidade exige a padronização do parâmetro \-x265-params open-gop=0 em fluxos baseados em libx265, garantindo estabilidade durante a navegação temporal e edições posteriores9. A manutenção do isolamento entre o processamento de vídeo e a cópia direta dos fluxos de áudio (-c:a copy) assegura a preservação de formatos de som surround complexos sem adicionar gargalos ao tempo de codificação11.

#### **Referências citadas**

> 1. ffmpeg-notes/ffmpeg-map.md at master · lingtalfi/ffmpeg-notes \- GitHub, [https://github.com/lingtalfi/ffmpeg-notes/blob/master/ffmpeg-map.md](https://github.com/lingtalfi/ffmpeg-notes/blob/master/ffmpeg-map.md)  
> 2. Re-encoding video with ffmpeg including all subtitles but not all audio, [https://unix.stackexchange.com/questions/245937/re-encoding-video-with-ffmpeg-including-all-subtitles-but-not-all-audio](https://unix.stackexchange.com/questions/245937/re-encoding-video-with-ffmpeg-including-all-subtitles-but-not-all-audio)  
> 3. Selecting streams with the \-map option, [https://trac.ffmpeg.org/wiki/Map](https://trac.ffmpeg.org/wiki/Map)  
> 4. FFmpeg not copying all audio streams \[closed\] \- Stack Overflow, [https://stackoverflow.com/questions/37820083/ffmpeg-not-copying-all-audio-streams](https://stackoverflow.com/questions/37820083/ffmpeg-not-copying-all-audio-streams)  
> 5. Docs/Ffmpeg.md \- Alliance for Open Media / SVT-AV1 · GitLab, [https://gitlab.com/AOMediaCodec/SVT-AV1/-/blob/master/Docs/Ffmpeg.md](https://gitlab.com/AOMediaCodec/SVT-AV1/-/blob/master/Docs/Ffmpeg.md)  
> 6. AV1 @ Scale: Film Grain Synthesis, The Awakening \- Netflix TechBlog, [https://netflixtechblog.com/av1-scale-film-grain-synthesis-the-awakening-ee09cfdff40b](https://netflixtechblog.com/av1-scale-film-grain-synthesis-the-awakening-ee09cfdff40b)  
> 7. ffmpeg Documentation, [https://ffmpeg.org/ffmpeg.html](https://ffmpeg.org/ffmpeg.html)  
> 8. Encode/H.264 – FFmpeg, [https://trac.ffmpeg.org/wiki/Encode/H.264](https://trac.ffmpeg.org/wiki/Encode/H.264)  
> 9. 11683 ("libx265" generated videos abnormal frames parsing?) \- FFmpeg Bug Tracker, [https://trac.ffmpeg.org/ticket/11683?cnum\_hist=4\&cversion=0](https://trac.ffmpeg.org/ticket/11683?cnum_hist=4&cversion=0)  
> 10. Encode/H.265 – FFmpeg, [https://trac.ffmpeg.org/wiki/Encode/H.265](https://trac.ffmpeg.org/wiki/Encode/H.265)  
> 11. How can I preserve all audio streams when cutting video files with FFmpeg? \- Ask Ubuntu, [https://askubuntu.com/questions/1526737/how-can-i-preserve-all-audio-streams-when-cutting-video-files-with-ffmpeg](https://askubuntu.com/questions/1526737/how-can-i-preserve-all-audio-streams-when-cutting-video-files-with-ffmpeg)  
> 12. Using ffmpeg on how do I copy the video and multiple subtitle streams in an MKV and reencode multiple audio streams into AC3? \- Super User, [https://superuser.com/questions/1273764/using-ffmpeg-on-how-do-i-copy-the-video-and-multiple-subtitle-streams-in-an-mkv](https://superuser.com/questions/1273764/using-ffmpeg-on-how-do-i-copy-the-video-and-multiple-subtitle-streams-in-an-mkv)  
> 13. Mapping included subtitles into video not working, at a loss : r/ffmpeg \- Reddit, [https://www.reddit.com/r/ffmpeg/comments/1c53f32/mapping\_included\_subtitles\_into\_video\_not\_working/](https://www.reddit.com/r/ffmpeg/comments/1c53f32/mapping_included_subtitles_into_video_not_working/)  
> 14. Discard synthesized film grain for VMAF calculation · Issue \#139 · alexheretic/ab-av1, [https://github.com/alexheretic/ab-av1/issues/139](https://github.com/alexheretic/ab-av1/issues/139)  
> 15. Turn on AV1 film grain synthesis and measure what it saves on your own footage, [https://dev.to/masonwritescode/turn-on-av1-film-grain-synthesis-and-measure-what-it-saves-on-your-own-footage-37bb](https://dev.to/masonwritescode/turn-on-av1-film-grain-synthesis-and-measure-what-it-saves-on-your-own-footage-37bb)  
> 16. Cannot use SVT-AV1 with neither Kdenlive or Shotcut (\#1958) · Issue \- GitLab, [https://gitlab.com/AOMediaCodec/SVT-AV1/-/issues/1958](https://gitlab.com/AOMediaCodec/SVT-AV1/-/issues/1958)  
> 17. 9610 (Sane defaults and full parameters for SVT AV1) \- FFmpeg Bug Tracker, [https://trac.ffmpeg.org/ticket/9610](https://trac.ffmpeg.org/ticket/9610)  
> 18. Releases · Alliance for Open Media / SVT-AV1 \- GitLab, [https://gitlab.com/AOMediaCodec/SVT-AV1/-/releases](https://gitlab.com/AOMediaCodec/SVT-AV1/-/releases)  
> 19. SVT-AV1/Docs/Ffmpeg.md at master \- GitHub, [https://github.com/AliveTeam/SVT-AV1/blob/master/Docs/Ffmpeg.md](https://github.com/AliveTeam/SVT-AV1/blob/master/Docs/Ffmpeg.md)  
> 20. Encode/AV1 \- FFmpeg Bug Tracker, [https://trac.ffmpeg.org/wiki/Encode/AV1](https://trac.ffmpeg.org/wiki/Encode/AV1)  
> 21. Newbie Questions About Using SVT-AV1 (libsvtav1) \- Reddit, [https://www.reddit.com/r/AV1/comments/10r9i2s/newbie\_questions\_about\_using\_svtav1\_libsvtav1/](https://www.reddit.com/r/AV1/comments/10r9i2s/newbie_questions_about_using_svtav1_libsvtav1/)  
> 22. Docs/Parameters.md · master · Alliance for Open Media / SVT-AV1 \- GitLab, [https://gitlab.com/AOMediaCodec/SVT-AV1/-/blob/master/Docs/Parameters.md](https://gitlab.com/AOMediaCodec/SVT-AV1/-/blob/master/Docs/Parameters.md)  
> 23. 6406 (Framerate issues with HVEC and mkv container) \- FFmpeg Bug Tracker, [https://trac.ffmpeg.org/ticket/6406](https://trac.ffmpeg.org/ticket/6406)  
> 24. HWAccelIntro – FFmpeg, [https://trac.ffmpeg.org/wiki/HWAccelIntro](https://trac.ffmpeg.org/wiki/HWAccelIntro)  
> 25. Is "libsvtav1 \-preset 5" supposed to be better than "libx265 slow"? : r/AV1 \- Reddit, [https://www.reddit.com/r/AV1/comments/y1kim3/is\_libsvtav1\_preset\_5\_supposed\_to\_be\_better\_than/](https://www.reddit.com/r/AV1/comments/y1kim3/is_libsvtav1_preset_5_supposed_to_be_better_than/)

[image1]: <data:image/png;base64,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>

[image2]: <data:image/png;base64,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>

[image3]: <data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAGAAAAAaCAYAAABIIVmfAAAFTUlEQVR4Xu1Ya8hmUxR+tsldogyS24jMIEV9URjDj0lfuSZ+jKJJIskPl1AihfxwiYxMCcldoZB7jUtuTYgQ8QMTaRr+DA1prLXXPmff1j7v3u/7ft98Pzy13nPOs9Z+9j577bPOPi8MCPZnASEbT0jEzizUoZWfFry+Oxuc4BI/bTT0Uwot8YOoblQd6KAsiEkkWjCY0HlB3HnNUGpiFj7mOdG1SPRvIXuk5/XO9ybbSM5DU0eCXiuCrrltoI8lzlQ2EVmjjFApHVHgErLNZI+FZAwbfz8dttJx6UA3SyhmhNYoDKgnWE6xb9DxNzIe2N9k75HtkGh86fz/kH1CdrLjV5B96HjyGx74OrJTnH8nsvfJ/hU/NpBd73y8yjYY4bntr3S8xPk68PUWF/MH2e2Jv8PzkD78pOVzMEO2BaK1NPGF6LVyCYsZ4nnONkG0voZcs/G9M/8w2V59i0FIL6sgYi9rIyfqLPr9jCx+dH3oO5D2B/YMIqU7yO7zlz32ALcz+DZ1BFgDSdb2qcPprwSv7JFPAN4luxcyzsM9Hd3vStNrmSEtxhMQraMi1mAZ/f4JSUg1dib7HfIELE58LPgWKS/2QzXhsPeEPAEfeYoR3Ri1tzoJb8830eGvxNHhWEhbxWepRWQfQMY8lIDzyW4luxauBHlXL12r1bX4ieznyOHxOdJEK3eQcg+6wV0WcPtAMrlvwKU4B9LZzQnfYVdICeuR9MsljNsfENM27HVIfVfyZg9coq5y55vpJ5k0G7gjpAzuAk6AUoKcXK8FJQHJmI+G6GhJ2g3yBLDx4oyg5aHDSRDRdfbK2Pr9EtnBPkQFlxZud3zqcJ2dSXZn5IjxJKT9iQnPC+HihAuxO+Sp60pTNmkO15GtdudqAhBouTGXtDpcA9HpdENcCvFx2cwwlADGN/B17UVIpmPkCl9Bytd2fJG7bYLOsGfKKoa8H7jP83oG2J/scTlVFAXcjt9NFvYJyCeNyyav/k6klIBeq5SAZBT8ZLJO+E7k1X4RZEPDmwU7H6MRK98EEebatoIJ/fYda3AIJP6F0JuAdwm8wkrglc4aVwfcoxgqewYH0e8rydiySSOsNX7HxqPWEmC1gmtGohX1xCWm201xeX6b7EdIyVlLdowPbcfZGHh8FFwBib8ydTjwCnmzlEbHnwbRuNuRF0DeK0N4huyIEQngiXgqidESYLXk1G0uhr8DTodolPyj4TrRpuUeiHhWzwt4ABI/I5exopFH+9yIzMG7I9Z4FrItvSvVUcATxG1KxrW5m2ze5nqfP/8BghqtEN1W9sKKcXpUhn6BoJ5X4EayrSbfCzOWG36PlDr2PL/s+Yb4A+8G4ofKVQn80cMag6vS+AWWvgNCqFrBbXwPSWS5REZQJqCjEhe/+Lhj/hLs4WOivX9HHUY/GyE31mER2eWQFc1bPx2x2HeQGso7sTZYHbMfZOz8cRTwGdZAnoAj7ZWLSUJzLY/unfdx6pgEp0JeJp9CxPktztfHhUEDoCTgIbKnyV4D13xjH1t9CnS8SnZbSg6jl38O8tR25WU92WzSO9Vt090fG/9dwOO1CEK9lth6cs7S8QTI9wp/qDL/C2SOZrOb1JPahtGNa3pRnHXUxChplniPmvtaIFhYY2wdTWu8Q00zJUahKjBeq/9h0TJ5LbHziPGHFbasKSllp6KUQGfnCK2dTRLf2lZBg0RD6ByguvfqwDJqJGpipoyxumxtpMVnnCMyflqwwg3qpdASbzHoLGPMZjqmIVatUR04AtPSaUDrgpgEc9nNdLQbVEqhJX4slMRKfAOcxH//lxN+DQsuegAAAABJRU5ErkJggg==>