import SwiftUI
import MLXLLM
import MLXLMCommon
import MLXHuggingFace
import Hub
import Tokenizers
import HuggingFace

private struct ChatMessage: Identifiable {
    let id = UUID()
    let role: String   // "user" | "assistant"
    var content: String
}

private enum GemmaModel: String, CaseIterable, Identifiable {
    case e2b = "E2B (2B)"
    case e4b = "E4B (4B)"
    var id: String { rawValue }
    var hfID: String {
        switch self {
        case .e2b: return "mlx-community/gemma-4-e2b-it-4bit"
        case .e4b: return "mlx-community/gemma-4-e4b-it-4bit"
        }
    }
}

struct GemmaChatView: View {
    @State private var selectedModel: GemmaModel = .e2b
    @State private var modelContainer: ModelContainer?
    @State private var isLoading = false
    @State private var loadProgress = ""
    @State private var messages: [ChatMessage] = []
    @State private var inputText = ""
    @State private var isGenerating = false
    @State private var streamingText = ""
    @State private var errorMessage = ""

    var isLoaded: Bool { modelContainer != nil }

    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                modelPickerBar

                if !isLoaded {
                    loadModelArea
                } else {
                    chatArea
                    inputBar
                }
            }
            .navigationTitle("Gemma 4 Chat")
            .navigationBarTitleDisplayMode(.inline)
        }
    }

    // MARK: - Sub-views

    private var modelPickerBar: some View {
        Picker("Model", selection: $selectedModel) {
            ForEach(GemmaModel.allCases) { m in
                Text(m.rawValue).tag(m)
            }
        }
        .pickerStyle(.segmented)
        .padding(.horizontal)
        .padding(.vertical, 8)
        .onChange(of: selectedModel) { _, _ in
            modelContainer = nil
            messages = []
            streamingText = ""
            errorMessage = ""
        }
    }

    private var loadModelArea: some View {
        VStack(spacing: 20) {
            Spacer()
            Image(systemName: "cpu")
                .font(.system(size: 60))
                .foregroundColor(.secondary)
            Text(selectedModel.rawValue)
                .font(.title2.bold())
            Text(selectedModel.hfID)
                .font(.caption)
                .foregroundColor(.secondary)
            if isLoading {
                ProgressView()
                Text(loadProgress)
                    .font(.caption)
                    .foregroundColor(.secondary)
                    .multilineTextAlignment(.center)
                    .padding(.horizontal)
            } else {
                Button(action: loadModel) {
                    Label("下载并加载模型", systemImage: "arrow.down.circle.fill")
                        .font(.headline)
                        .padding(.horizontal, 28)
                        .padding(.vertical, 14)
                        .background(Color.indigo)
                        .foregroundColor(.white)
                        .clipShape(Capsule())
                }
                if !errorMessage.isEmpty {
                    Text(errorMessage)
                        .font(.caption)
                        .foregroundColor(.red)
                        .padding(.horizontal)
                }
            }
            Spacer()
        }
    }

    private var chatArea: some View {
        ScrollViewReader { proxy in
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 12) {
                    ForEach(messages) { msg in
                        messageBubble(msg)
                            .id(msg.id)
                    }
                    if isGenerating && !streamingText.isEmpty {
                        streamingBubble
                            .id("streaming")
                    }
                }
                .padding()
            }
            .onChange(of: streamingText) { _, _ in
                withAnimation { proxy.scrollTo("streaming", anchor: .bottom) }
            }
            .onChange(of: messages.count) { _, _ in
                if let last = messages.last {
                    withAnimation { proxy.scrollTo(last.id, anchor: .bottom) }
                }
            }
        }
    }

    private func messageBubble(_ msg: ChatMessage) -> some View {
        HStack {
            if msg.role == "user" { Spacer(minLength: 60) }
            Text(msg.content)
                .padding(.horizontal, 14)
                .padding(.vertical, 10)
                .background(msg.role == "user" ? Color.indigo : Color(.secondarySystemBackground))
                .foregroundColor(msg.role == "user" ? .white : .primary)
                .clipShape(RoundedRectangle(cornerRadius: 16))
            if msg.role == "assistant" { Spacer(minLength: 60) }
        }
    }

    private var streamingBubble: some View {
        HStack {
            Text(streamingText)
                .padding(.horizontal, 14)
                .padding(.vertical, 10)
                .background(Color(.secondarySystemBackground))
                .clipShape(RoundedRectangle(cornerRadius: 16))
            Spacer(minLength: 60)
        }
    }

    private var inputBar: some View {
        HStack(spacing: 10) {
            TextField("输入消息...", text: $inputText, axis: .vertical)
                .lineLimit(1...4)
                .padding(10)
                .background(Color(.secondarySystemBackground))
                .clipShape(RoundedRectangle(cornerRadius: 12))
            Button(action: sendMessage) {
                Image(systemName: isGenerating ? "stop.fill" : "arrow.up.circle.fill")
                    .font(.system(size: 32))
                    .foregroundColor(inputText.isEmpty && !isGenerating ? .gray : .indigo)
            }
            .disabled(inputText.isEmpty || isGenerating)
        }
        .padding(.horizontal)
        .padding(.vertical, 10)
        .background(.bar)
    }

    // MARK: - Actions

    private func loadModel() {
        isLoading = true
        errorMessage = ""
        loadProgress = "准备下载..."
        let config = ModelConfiguration(id: selectedModel.hfID)
        Task {
            do {
                let container = try await #huggingFaceLoadModelContainer(
                    configuration: config,
                    progressHandler: { progress in
                        Task { @MainActor in
                            let pct = Int(progress.fractionCompleted * 100)
                            loadProgress = "下载中... \(pct)%"
                        }
                    }
                )
                await MainActor.run {
                    modelContainer = container
                    isLoading = false
                    loadProgress = ""
                }
            } catch {
                await MainActor.run {
                    errorMessage = error.localizedDescription
                    isLoading = false
                }
            }
        }
    }

    private func sendMessage() {
        guard let container = modelContainer,
              !inputText.trimmingCharacters(in: .whitespaces).isEmpty else { return }
        let text = inputText
        inputText = ""
        messages.append(ChatMessage(role: "user", content: text))
        isGenerating = true
        streamingText = ""

        let history = messages.map { ["role": $0.role, "content": $0.content] }

        Task {
            do {
                let reply = try await container.perform { context in
                    let input = try await context.processor.prepare(
                        input: .init(messages: history)
                    )
                    var params = GenerateParameters()
                    params.maxTokens = 512

                    let stream = try generate(
                        input: input, parameters: params, context: context)

                    var output = ""
                    for await generation in stream {
                        if case .chunk(let chunk) = generation {
                            output += chunk
                            Task { @MainActor in self.streamingText = output }
                        }
                    }
                    return output
                }
                await MainActor.run {
                    messages.append(ChatMessage(role: "assistant", content: reply))
                    streamingText = ""
                    isGenerating = false
                }
            } catch {
                await MainActor.run {
                    messages.append(ChatMessage(
                        role: "assistant",
                        content: "错误: \(error.localizedDescription)"))
                    streamingText = ""
                    isGenerating = false
                }
            }
        }
    }
}
